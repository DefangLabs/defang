package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/agent"
	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/cluster"
	"github.com/DefangLabs/defang/src/pkg/debug"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/github"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/mcp"
	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/setup"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/surveyor"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/timeutils"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

const authNeeded = "auth-needed" // annotation to indicate that a command needs authorization
var authNeededAnnotation = map[string]string{authNeeded: ""}

var P = track.P

// getTenantSelection resolves the tenant to use for this invocation (flag > env > token subject),
// leaving it unset when we should rely on the personal tenant from the token subject.
func getTenantSelection() types.TenantNameOrID {
	if global.Tenant != "" {
		return types.TenantNameOrID(global.Tenant)
	}
	if token := cluster.GetExistingToken(global.Cluster); token != "" {
		if t := cli.TenantFromToken(token); t.IsSet() {
			return t
		}
	}
	// No explicit tenant: defer to token subject or server defaults.
	return types.TenantUnset
}

func Execute(ctx context.Context) error {
	if term.StdoutCanColor() {
		restore := term.EnableANSI()
		defer restore()
	}

	if err := RootCmd.ExecuteContext(ctx); err != nil {
		if !(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			term.Error("Error:", cliClient.PrettyError(err))
		}

		if err == dryrun.ErrDryRun {
			return nil
		}

		if cerr := new(cli.ComposeError); errors.As(err, &cerr) {
			compose := "compose"
			fileFlag := RootCmd.Flag("file")
			if fileFlag.Changed {
				compose += " -f " + fileFlag.Value.String()
			}
			printDefangHint("Fix the error and try again. To validate the compose file, use:", compose+" config")
		}

		if strings.Contains(err.Error(), "config") {
			printDefangHint("To manage sensitive service config, use:", "config")
		}

		if strings.Contains(err.Error(), "maximum number of projects") {
			projectName := "<name>"
			provider, err := newProviderChecked(ctx, nil)
			if err != nil {
				return err
			}
			if resp, err := provider.RemoteProjectName(ctx); err == nil {
				projectName = resp
			}
			printDefangHint("To deactivate a project, do:", "compose down --project-name "+projectName)
		}

		if cerr := new(cli.CancelError); errors.As(err, &cerr) {
			printDefangHint("Detached. The deployment will keep running.\nTo continue the logs from where you left off, do:", cerr.Error())
		}

		code := connect.CodeOf(err)
		if code == connect.CodeUnauthenticated {
			printDefangHint("Please use the following command to log in:", "login")
		}
		if code == connect.CodeFailedPrecondition && (strings.Contains(err.Error(), "EULA") || strings.Contains(err.Error(), "terms")) {
			printDefangHint("Please use the following command to see the Defang terms of service:", "terms")
		}

		if cde := new(gcp.ConflictDelegateDomainError); errors.As(err, cde) {
			hint := fmt.Sprintf("Domain verification is required for the delegated domain %q, as indicated by the Google Cloud API.", cde.NewDomain)
			if cde.ConflictDomain != "" {
				hint += fmt.Sprintf(" This is likely due to a legacy tenant-level delegated zone named %v (%v).", cde.ConflictZone, cde.ConflictDomain)
			} else {
				hint += " This is likely caused by a conflicting legacy tenant-level delegated DNS zone."
			}
			hint += " Please check if this legacy zone is still needed. If not, remove it from your Google Cloud account and try again."
			printDefangHint(hint)
		}

		if credError := new(gcp.CredentialsError); errors.As(err, &credError) {
			term.Print("\nPlease log in by running: \n\n\t gcloud auth application-default login\n\n")
		}

		return ExitCode(code)
	}

	if global.HasTty && term.HadWarnings() {
		term.Println("For help with warnings, check our FAQ at https://s.defang.io/warnings")
	}

	if global.HasTty && !global.HideUpdate && pkg.RandomIndex(10) == 0 {
		if latest, err := github.GetLatestReleaseTag(ctx); err == nil && isNewer(GetCurrentVersion(), latest) {
			term.Debug("Latest Version:", latest, "Current Version:", GetCurrentVersion())
			term.Println("A newer version of the CLI is available at https://github.com/DefangLabs/defang/releases/latest")
			if pkg.RandomIndex(10) == 0 && !pkg.GetenvBool("DEFANG_HIDE_HINTS") {
				term.Println("To silence these notices, do: export DEFANG_HIDE_UPDATE=1")
			}
		}
	}

	return nil
}

/*
SetupCommands initializes and configures the entire Defang CLI command structure.
It registers all global flags that bind to GlobalConfig, sets up all subcommands with their
specific flags, and establishes the command hierarchy.
*/
func SetupCommands(ctx context.Context, version string) {
	cobra.EnableTraverseRunHooks = true // we always need to run the RootCmd's pre-run hook

	RootCmd.Version = version
	RootCmd.PersistentFlags().StringVarP(&global.Stack.Name, "stack", "s", global.Stack.Name, "stack name (for BYOC providers)")
	RootCmd.RegisterFlagCompletionFunc("stack", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		stacks, err := stacks.List()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var completions []cobra.Completion
		for _, stack := range stacks {
			completions = append(completions, stack.Name)
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	})
	RootCmd.PersistentFlags().Var(&global.ColorMode, "color", fmt.Sprintf(`colorize output; one of %v`, allColorModes))
	RootCmd.RegisterFlagCompletionFunc("color", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var completions []cobra.Completion
		for _, mode := range allColorModes {
			completions = append(completions, mode.String())
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	})
	RootCmd.PersistentFlags().StringVar(&global.Cluster, "cluster", global.Cluster, "Defang cluster to connect to")
	RootCmd.PersistentFlags().MarkHidden("cluster") // only for Defang use
	RootCmd.PersistentFlags().StringVar(&global.Tenant, "workspace", global.Tenant, "workspace to use (tenant name or ID)")
	RootCmd.PersistentFlags().VarP(&global.Stack.Provider, "provider", "P", fmt.Sprintf(`bring-your-own-cloud provider; one of %v`, cliClient.AllProviders()))
	RootCmd.RegisterFlagCompletionFunc("provider", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var completions []cobra.Completion
		for _, provider := range cliClient.AllProviders() {
			completions = append(completions, provider.String())
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	})
	// RootCmd.Flag("provider").NoOptDefVal = "auto" NO this will break the "--provider aws"
	RootCmd.Flags().MarkDeprecated("provider", "please use --stack instead")
	RootCmd.PersistentFlags().BoolVarP(&global.Verbose, "verbose", "v", global.Verbose, "verbose logging") // backwards compat: only used by tail
	RootCmd.PersistentFlags().BoolVar(&global.Debug, "debug", global.Debug, "debug logging for troubleshooting the CLI")
	RootCmd.PersistentFlags().BoolVar(&dryrun.DoDryRun, "dry-run", false, "dry run (don't actually change anything)")
	RootCmd.PersistentFlags().BoolVar(&global.NonInteractive, "non-interactive", global.NonInteractive, "disable interactive prompts / no TTY")
	RootCmd.PersistentFlags().StringP("project-name", "p", "", "project name")
	RootCmd.PersistentFlags().StringP("cwd", "C", "", "change directory before running the command")
	_ = RootCmd.MarkPersistentFlagDirname("cwd")
	RootCmd.PersistentFlags().StringArrayP("file", "f", []string{}, `compose file path(s)`)
	_ = RootCmd.MarkPersistentFlagFilename("file", "yml", "yaml")

	// Create a temporary gRPC client for tracking events before login
	// getTenantSelection defaults to types.TenantUnset when no tenant is specified
	cli.ConnectWithTenant(ctx, global.Cluster, getTenantSelection())

	// CD command
	RootCmd.AddCommand(cdCmd)
	cdCmd.PersistentFlags().Bool("utc", false, "show logs in UTC timezone (ie. TZ=UTC)")
	cdCmd.PersistentFlags().Bool("json", pkg.GetenvBool("DEFANG_JSON"), "show logs in JSON format")
	cdCmd.PersistentFlags().StringVar(&byoc.DefangPulumiBackend, "pulumi-backend", "", `specify an alternate Pulumi backend URL or "pulumi-cloud"`)
	cdCmd.AddCommand(cdDestroyCmd)
	cdCmd.AddCommand(cdDownCmd)
	cdCmd.AddCommand(cdRefreshCmd)
	cdTearDownCmd.Flags().Bool("force", false, "force the teardown of the CD stack")
	cdCmd.AddCommand(cdTearDownCmd)
	cdListCmd.Flags().BoolP("all", "a", false, "list projects and stacks in all regions")
	cdListCmd.Flags().Bool("remote", false, "invoke the command on the remote cluster")
	cdCmd.AddCommand(cdListCmd)
	cdCmd.AddCommand(cdCancelCmd)
	cdPreviewCmd.Flags().VarP(&global.Stack.Mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", modes.AllDeploymentModes()))
	cdPreviewCmd.RegisterFlagCompletionFunc("mode", cobra.FixedCompletions(modes.AllDeploymentModes(), cobra.ShellCompDirectiveNoFileComp))
	cdCmd.AddCommand(cdPreviewCmd)
	cdCmd.AddCommand(cdInstallCmd)
	cdCmd.AddCommand(cdCloudformationCmd)

	// Eula command
	tosCmd.Flags().Bool("agree-tos", false, "agree to the Defang terms of service")
	RootCmd.AddCommand(tosCmd)

	// Upgrade command
	RootCmd.AddCommand(upgradeCmd)

	// Token command
	tokenCmd.Flags().Duration("expires", 24*time.Hour, "validity duration of the token")
	tokenCmd.Flags().String("scope", "", fmt.Sprintf("scope of the token; one of %v (required)", scope.All())) // TODO: make it an Option
	_ = tokenCmd.MarkFlagRequired("scope")
	tokenCmd.RegisterFlagCompletionFunc("scope", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var completions []cobra.Completion
		for _, s := range scope.All() {
			completions = append(completions, s.String())
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	})
	RootCmd.AddCommand(tokenCmd)

	// Login Command
	loginCmd.Flags().Bool("training-opt-out", false, "Opt out of ML training (Pro users only)")
	// loginCmd.Flags().Bool("skip-prompt", false, "skip the login prompt if already logged in"); TODO: Implement this
	RootCmd.AddCommand(loginCmd)

	// Whoami Command
	whoamiCmd.PersistentFlags().Bool("json", pkg.GetenvBool("DEFANG_JSON"), "print output in JSON format")
	RootCmd.AddCommand(whoamiCmd)

	// Workspace Command
	RootCmd.AddCommand(workspaceCmd)

	// Logout Command
	RootCmd.AddCommand(logoutCmd)

	// Generate Command
	generateCmd.Flags().StringVar(&global.ModelID, "model", global.ModelID, "LLM model to use for generating the code (Pro users only)")
	RootCmd.AddCommand(generateCmd)
	// new command
	initCmd.PersistentFlags().Var(&global.SourcePlatform, "from", fmt.Sprintf(`the platform from which to migrate the project; one of %v`, migrate.AllSourcePlatforms))
	RootCmd.AddCommand(initCmd)

	// Get Services Command
	lsCommand := makeComposePsCmd()
	lsCommand.Use = "services"
	// TODO: when we add multi-project support to the playground, differentiate
	// between ls and ps
	lsCommand.Aliases = []string{"getServices", "ps", "ls", "list"}
	RootCmd.AddCommand(lsCommand)

	// Version Command
	RootCmd.AddCommand(versionCmd)

	// Config Command (was: secrets)
	configSetCmd.Flags().BoolP("name", "n", false, "name of the config (backwards compat)")
	configSetCmd.Flags().BoolP("env", "e", false, "set the config from an environment variable")
	configSetCmd.Flags().Bool("random", false, "set a secure randomly generated value for config")
	configSetCmd.Flags().String("env-file", "", "load config values from an .env file")
	configSetCmd.MarkFlagFilename("env-file")
	_ = configSetCmd.Flags().MarkHidden("name")

	configCmd.AddCommand(configSetCmd)

	configDeleteCmd.Flags().BoolP("name", "n", false, "name of the config(s) (backwards compat)")
	_ = configDeleteCmd.Flags().MarkHidden("name")
	configCmd.AddCommand(configDeleteCmd)

	configCmd.AddCommand(configListCmd)

	RootCmd.AddCommand(configCmd)

	RootCmd.AddCommand(setupComposeCommand())
	// Add up/down commands to the root as well
	down := makeComposeDownCmd()
	down.Hidden = true // hidden from top-level menu
	RootCmd.AddCommand(down)
	up := makeComposeUpCmd()
	up.Hidden = true // hidden from top-level menu
	RootCmd.AddCommand(up)
	restart := makeComposeRestartCmd()
	restart.Hidden = true // hidden from top-level menu
	RootCmd.AddCommand(restart)

	estimateCmd := makeEstimateCmd()
	RootCmd.AddCommand(estimateCmd)

	// Debug Command
	debugCmd.Flags().String("etag", "", "deployment ID (ETag) of the service")
	debugCmd.Flags().MarkHidden("etag")
	debugCmd.Flags().String("deployment", "", "deployment ID of the service")
	debugCmd.Flags().String("since", "", "start time for logs (RFC3339 format)")
	debugCmd.Flags().String("until", "", "end time for logs (RFC3339 format)")
	debugCmd.Flags().StringVar(&global.ModelID, "model", global.ModelID, "LLM model to use for debugging (Pro users only)")
	RootCmd.AddCommand(debugCmd)

	// Tail Command
	tailCmd := makeTailCmd()
	RootCmd.AddCommand(tailCmd)

	// Logs Command
	logsCmd := makeLogsCmd()
	RootCmd.AddCommand(logsCmd)

	// Delete Command
	deleteCmd.Flags().BoolP("name", "n", false, "name of the service(s) (backwards compat)")
	_ = deleteCmd.Flags().MarkHidden("name")
	deleteCmd.Flags().Bool("tail", false, "tail the service logs after deleting")
	RootCmd.AddCommand(deleteCmd)

	// Deployments Command
	deploymentsCmd.AddCommand(deploymentsListCmd)
	deploymentsCmd.PersistentFlags().Bool("utc", false, "show logs in UTC timezone (ie. TZ=UTC)")
	RootCmd.AddCommand(deploymentsCmd)

	// MCP Command
	mcpServerCmd.Flags().Int("auth-server", 0, "auth server port")
	mcpServerCmd.Flags().MarkDeprecated("auth-server", "we now reach out to the auth server: https://auth.defang.io directly")
	mcpCmd.AddCommand(mcpServerCmd)
	mcpCmd.PersistentFlags().String("client", "", fmt.Sprintf("MCP setup client %v", mcp.ValidClients))
	_ = mcpCmd.RegisterFlagCompletionFunc("client", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var completions []string
		for _, client := range mcp.ValidClients {
			completions = append(completions, string(client))
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	})
	mcpCmd.AddCommand(mcpSetupCmd)
	RootCmd.AddCommand(mcpCmd)

	// Send Command
	sendCmd.Flags().StringP("subject", "n", "", "subject to send the message to (required)")
	sendCmd.Flags().StringP("type", "t", "", "type of message to send (required)")
	sendCmd.Flags().String("id", "", "ID of the message")
	sendCmd.Flags().StringP("data", "d", "", "string data to send")
	sendCmd.Flags().StringP("content-type", "c", "", "Content-Type of the data")
	_ = sendCmd.MarkFlagRequired("subject")
	_ = sendCmd.MarkFlagRequired("type")
	RootCmd.AddCommand(sendCmd)

	// Cert management
	// TODO: Add list, renew etc.
	certCmd.AddCommand(certGenerateCmd)
	RootCmd.AddCommand(certCmd)

	stackCmd := makeStackCmd()
	RootCmd.AddCommand(stackCmd)

	if term.StdoutCanColor() { // TODO: should use DoColor(…) instead
		// Add some emphasis to the help command
		re := regexp.MustCompile(`(?m)^[A-Za-z ]+?:`)
		templ := re.ReplaceAllString(RootCmd.UsageTemplate(), "\033[1m$0\033[0m") // bold
		RootCmd.SetUsageTemplate(templ)
	}

	origHelpFunc := RootCmd.HelpFunc()
	RootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		track.Cmd(cmd, "Help", P("args", args))
		origHelpFunc(cmd, args)
	})
}

func getCwd(args []string) string {
	for i, arg := range args {
		if i > 0 && (args[i-1] == "--cwd" || args[i-1] == "-C") {
			return arg
		} else if dir, ok := strings.CutPrefix(arg, "-C="); ok {
			return dir
		} else if dir, ok := strings.CutPrefix(arg, "--cwd="); ok {
			return dir
		}
	}
	return ""
}

var RootCmd = &cobra.Command{
	SilenceUsage:  true,
	SilenceErrors: true,
	Use:           "defang",
	Args:          cobra.NoArgs,
	Short:         "Defang CLI is used to take your app from Docker Compose to a secure and scalable deployment on your favorite cloud in minutes.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		// Don't track/connect the shell completion commands
		if IsCompletionCommand(cmd) {
			// but do change the directory for file completions to work correctly.
			// Unfortunately, Cobra will not have parsed the "cwd" flag.
			if cwd := getCwd(os.Args); cwd != "" {
				return os.Chdir(cwd)
			}
			return nil
		}

		ctx := cmd.Context()
		term.SetDebug(global.Debug)

		// Use "defer" to track any errors that occur during the command
		defer func() {
			var errString = ""
			if err != nil {
				errString = err.Error()
			}

			track.Cmd(cmd, "Invoked", P("args", args), P("err", errString), P("non-interactive", global.NonInteractive), P("provider", global.Stack.Provider))
		}()

		// Do this first, since any errors will be printed to the console
		switch global.ColorMode {
		case ColorNever:
			term.ForceColor(false)
		case ColorAlways:
			term.ForceColor(true)
		}

		if cwd, _ := cmd.Flags().GetString("cwd"); cwd != "" {
			// Change directory before running the command
			if err = os.Chdir(cwd); err != nil {
				return err
			}
		}

		// Read the global flags again from any .defang files in the cwd
		err = loadStackFile(global.getStackName(cmd.Flags()))
		if err != nil {
			return err
		}

		err = global.syncFlagsWithEnv(cmd.Flags())
		if err != nil {
			return err
		}

		global.Client, err = cli.ConnectWithTenant(ctx, global.Cluster, getTenantSelection())

		if err != nil {
			if connect.CodeOf(err) != connect.CodeUnauthenticated {
				return err
			}
			term.Debug("Using existing token failed; continuing to allow login/ToS flow:", err)
		}

		if v, err := global.Client.GetVersions(ctx); err == nil {
			version := cmd.Root().Version // HACK to avoid circular dependency with RootCmd
			term.Debug("Fabric:", v.Fabric, "CLI:", version, "CLI-Min:", v.CliMin)
			if global.HasTty && isNewer(version, v.CliMin) && !isUpgradeCommand(cmd) {
				term.Warn("Your CLI version is outdated. Please upgrade to the latest version by running:\n\n  defang upgrade\n")
				global.HideUpdate = true // hide the upgrade hint at the end
			}
		}

		// Check if we are correctly logged in, but only if the command needs authorization
		if _, ok := cmd.Annotations[authNeeded]; !ok {
			return nil
		}

		if global.NonInteractive {
			err = global.Client.CheckLoginAndToS(ctx)
		} else {
			err = login.InteractiveRequireLoginAndToS(ctx, global.Client, global.Cluster)
		}

		return err
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if global.NonInteractive {
			return cmd.Help()
		}

		ctx := cmd.Context()
		err := login.InteractiveRequireLoginAndToS(ctx, global.Client, global.Cluster)
		if err != nil {
			return err
		}

		prompt := "Welcome to Defang. I can help you deploy your project to the cloud."
		ag, err := agent.New(ctx, global.Cluster, &global.Stack)
		if err != nil {
			return err
		}
		return ag.StartWithUserPrompt(ctx, prompt)
	},
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Args:  cobra.NoArgs,
	Short: "Authenticate to Defang",
	RunE: func(cmd *cobra.Command, args []string) error {
		trainingOptOut, _ := cmd.Flags().GetBool("training-opt-out")

		if global.NonInteractive {
			if err := login.NonInteractiveGitHubLogin(cmd.Context(), global.Client, global.Cluster); err != nil {
				return err
			}
		} else {
			err := login.InteractiveLogin(cmd.Context(), global.Client, global.Cluster)
			if err != nil {
				return err
			}

			printDefangHint("To generate a sample service, do:", "generate")
		}

		if trainingOptOut {
			req := &defangv1.SetOptionsRequest{TrainingOptOut: trainingOptOut}
			if err := global.Client.SetOptions(cmd.Context(), req); err != nil {
				return err
			}
			term.Info("Options updated successfully")
		}
		return nil
	},
}

var whoamiCmd = &cobra.Command{
	Use:         "whoami",
	Args:        cobra.NoArgs,
	Short:       "Show the current user",
	Annotations: authNeededAnnotation,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonMode, _ := cmd.Flags().GetBool("json")

		loader := configureLoader(cmd)

		global.NonInteractive = true // don't show provider prompt
		ctx := cmd.Context()
		projectName, err := loader.LoadProjectName(ctx)
		if err != nil {
			term.Warnf("Unable to load project: %v", err)
		}
		elicitationsClient := elicitations.NewSurveyClient(os.Stdin, os.Stdout, os.Stderr)
		ec := elicitations.NewController(elicitationsClient)
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
		sm, err := stacks.NewManager(global.Client, wd, projectName)
		if err != nil {
			return fmt.Errorf("failed to create stack manager: %w", err)
		}
		provider, err := newProvider(cmd.Context(), ec, sm)
		if err != nil {
			term.Debug("unable to get provider:", err)
		}

		token := cluster.GetExistingToken(global.Cluster)

		userInfo, err := auth.FetchUserInfo(cmd.Context(), token)
		if err != nil {
			// Either the auth service is down, or we're using a Fabric JWT: skip workspace information
			if !jsonMode {
				term.Warn("Workspace information unavailable:", err)
			}
		}

		tenantSelection := getTenantSelection()
		data, err := cli.Whoami(cmd.Context(), global.Client, provider, userInfo, tenantSelection)
		if err != nil {
			return err
		}

		if !global.Verbose {
			data.Tenant = ""
			data.TenantID = ""
		}

		if jsonMode {
			bytes, err := json.Marshal(data)
			if err != nil {
				return err
			}
			_, err = term.Println(string(bytes))
			return err
		} else {
			cols := []string{
				"Workspace",
				"SubscriberTier",
				"Name",
				"Email",
				"Provider",
				"Region",
			}
			if global.Verbose {
				cols = append(cols, "Tenant", "TenantID")
			}
			return term.Table([]cli.ShowAccountData{data}, cols...)
		}
	},
}

var certCmd = &cobra.Command{
	Use:   "cert",
	Args:  cobra.NoArgs,
	Short: "Manage certificates",
}

var certGenerateCmd = &cobra.Command{
	Use:     "generate",
	Aliases: []string{"gen"},
	Args:    cobra.NoArgs,
	Short:   "Generate a TLS certificate",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		project, err := loader.LoadProject(cmd.Context())
		if err != nil {
			return err
		}

		provider, err := newProviderChecked(cmd.Context(), loader)
		if err != nil {
			return err
		}

		if err := cli.GenerateLetsEncryptCert(cmd.Context(), project, global.Client, provider); err != nil {
			return err
		}
		return nil
	},
}

func afterGenerate(ctx context.Context, result setup.SetupResult) {
	term.Info("Code generated successfully in folder", result.Folder)
	editor := pkg.Getenv("DEFANG_EDITOR", "code") // TODO: should we use EDITOR env var instead? But won't handle terminal editors like vim
	cmdd := exec.Command(editor, result.Folder)
	err := cmdd.Start()
	if err != nil {
		term.Debugf("unable to launch editor %q: %v", editor, err)
	}

	cd := ""
	if result.Folder != "." {
		cd = "`cd " + result.Folder + "` and "
	}

	// Load the project and check for empty environment variables
	loader := compose.NewLoader(compose.WithPath(filepath.Join(result.Folder, "compose.yaml")))
	project, err := loader.LoadProject(ctx)
	if err != nil {
		term.Debugf("unable to load new project: %v", err)
	}

	var envInstructions []string
	for _, envVar := range collectUnsetEnvVars(project) {
		envInstructions = append(envInstructions, "config create "+envVar)
	}

	if len(envInstructions) > 0 {
		printDefangHint("Check the files in your favorite editor.\nTo configure this project, run "+cd, envInstructions...)
	} else {
		printDefangHint("Check the files in your favorite editor.\nTo deploy this project, run "+cd, "compose up")
	}
}

var generateCmd = &cobra.Command{
	Use:     "generate",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"gen"},
	Short:   "Generate a sample Defang project",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		if global.NonInteractive {
			if len(args) == 0 {
				return errors.New("cannot run in non-interactive mode")
			}
			return cli.InitFromSamples(ctx, args[0], args)
		}

		setupClient := setup.SetupClient{
			Surveyor: surveyor.NewDefaultSurveyor(),
			Heroku:   migrate.NewHerokuClient(),
			ModelID:  global.ModelID,
			Fabric:   global.Client,
			Cluster:  global.Cluster,
		}

		var sample string
		if len(args) > 0 {
			sample = args[0]
		}
		result, err := setupClient.CloneSample(ctx, sample)
		if err != nil {
			return err
		}
		afterGenerate(ctx, result)
		return nil
	},
}

var initCmd = &cobra.Command{
	Use:     "init [SAMPLE]",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"new"},
	Short:   "Create a new Defang project from a sample",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		if global.NonInteractive {
			if len(args) == 0 {
				return errors.New("cannot run in non-interactive mode")
			}
			return cli.InitFromSamples(ctx, args[0], args)
		}

		setupClient := setup.SetupClient{
			Surveyor: surveyor.NewDefaultSurveyor(),
			Heroku:   migrate.NewHerokuClient(),
			ModelID:  global.ModelID,
			Fabric:   global.Client,
			Cluster:  global.Cluster,
		}

		if len(args) > 0 {
			_, err := setupClient.CloneSample(ctx, args[0])
			return err
		}

		result, err := setupClient.Start(ctx)
		if err != nil {
			return err
		}
		afterGenerate(ctx, result)
		return nil
	},
}

func collectUnsetEnvVars(project *composeTypes.Project) []string {
	if project == nil {
		return nil // in case loading failed
	}
	err := compose.ValidateProjectConfig(context.TODO(), project, func(ctx context.Context) ([]string, error) {
		return nil, nil // assume no config
	})
	var missingConfig compose.ErrMissingConfig
	if errors.As(err, &missingConfig) {
		return missingConfig
	}
	return nil
}

var versionCmd = &cobra.Command{
	Use:     "version",
	Args:    cobra.NoArgs,
	Aliases: []string{"ver", "stat", "status"}, // for backwards compatibility
	Short:   "Get version information for the CLI and Fabric service",
	RunE: func(cmd *cobra.Command, args []string) error {
		term.Printc(term.BrightCyan, "Defang CLI:    ")
		term.Println(GetCurrentVersion())

		term.Printc(term.BrightCyan, "Latest CLI:    ")
		ver, err := github.GetLatestReleaseTag(cmd.Context())
		term.Println(ver)

		term.Printc(term.BrightCyan, "Defang Fabric: ")
		ver, err2 := cli.GetVersion(cmd.Context(), global.Client)
		term.Println(ver)
		return errors.Join(err, err2)
	},
}

var configCmd = &cobra.Command{
	Use:     "config", // like Docker
	Args:    cobra.NoArgs,
	Aliases: []string{"secrets", "secret"},
	Short:   "Add, update, or delete service config",
}

var configSetCmd = &cobra.Command{
	Use:         "create CONFIG [file|-]", // like Docker
	Annotations: authNeededAnnotation,
	Args:        cobra.RangeArgs(0, 2), // Allow 0 args when using --env-file
	Aliases:     []string{"set", "add", "put"},
	Short:       "Adds or updates a sensitive config value",
	RunE: func(cmd *cobra.Command, args []string) error {
		fromEnv, _ := cmd.Flags().GetBool("env")
		random, _ := cmd.Flags().GetBool("random")
		envFile, _ := cmd.Flags().GetString("env-file")

		// Make sure we have a project to set config for before asking for a value
		loader := configureLoader(cmd)
		provider, err := newProviderChecked(cmd.Context(), loader)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
		if err != nil {
			return err
		}

		// Handle --env-file flag
		if envFile != "" {
			if fromEnv || random {
				return errors.New("cannot use --env-file with --env or --random")
			}
			if len(args) > 0 {
				return errors.New("cannot specify CONFIG arguments with --env-file")
			}

			envMap, err := godotenv.Read(envFile)
			if err != nil {
				return fmt.Errorf("failed to read env file %q: %w", envFile, err)
			}

			if len(envMap) == 0 {
				return errors.New("no config found in env file")
			}

			// Set each config from the env file
			successCount := 0
			for name, value := range envMap {
				if !pkg.IsValidSecretName(name) {
					term.Warnf("Skipping invalid config name: %q", name)
					continue
				}

				if err := cli.ConfigSet(cmd.Context(), projectName, provider, name, value); err != nil {
					term.Warnf("Failed to set %q: %v", name, err)
				} else {
					term.Info("Updated value for", name)
					successCount++
				}
			}

			if successCount == 0 {
				return errors.New("failed to set any config values")
			}

			term.Infof("Successfully set %d config value(s)", successCount)

			printDefangHint("To update the deployed values, do:", "compose up")
			return nil
		}

		// Original single config logic
		if len(args) == 0 {
			return errors.New("CONFIG argument is required when not using --env-file")
		}

		parts := strings.SplitN(args[0], "=", 2)
		name := parts[0]

		if !pkg.IsValidSecretName(name) {
			return fmt.Errorf("invalid config name: %q", name)
		}

		var value string
		if fromEnv {
			if len(args) == 2 || len(parts) == 2 {
				return errors.New("cannot specify config value or input file when using --env")
			}
			var ok bool
			value, ok = os.LookupEnv(name)
			if !ok {
				return fmt.Errorf("environment variable %q not found", name)
			}
		} else if len(parts) == 2 {
			// Handle name=value; can't also specify a file in this case
			if len(args) == 2 {
				return errors.New("cannot specify both config value and input file")
			}
			value = parts[1]
		} else if global.NonInteractive || len(args) == 2 {
			// Read the value from a file or stdin
			var err error
			var bytes []byte
			if len(args) == 2 && args[1] != "-" {
				bytes, err = os.ReadFile(args[1])
			} else {
				bytes, err = io.ReadAll(os.Stdin)
			}
			if err != nil && err != io.EOF {
				return fmt.Errorf("failed reading the config value: %w", err)
			}
			// Trim the newline at the end because single line values are common
			value = strings.TrimSuffix(string(bytes), "\n")
		} else if random {
			// Generate a random value for the config
			value = cli.CreateRandomConfigValue()
			term.Info("Generated random value: " + value)
		} else {
			// Prompt for sensitive value
			var sensitivePrompt = &survey.Password{
				Message: fmt.Sprintf("Enter value for %q:", name),
				Help:    "The value will be stored securely and cannot be retrieved later.",
			}

			err := survey.AskOne(sensitivePrompt, &value, survey.WithStdio(term.DefaultTerm.Stdio()))
			if err != nil {
				return err
			}
		}

		if err := cli.ConfigSet(cmd.Context(), projectName, provider, name, value); err != nil {
			return err
		}
		term.Info("Updated value for", name)

		printDefangHint("To update the deployed values, do:", "compose up")
		return nil
	},
}

var configDeleteCmd = &cobra.Command{
	Use:         "rm CONFIG...", // like Docker
	Annotations: authNeededAnnotation,
	Args:        cobra.MinimumNArgs(1),
	Aliases:     []string{"del", "delete", "remove"},
	Short:       "Removes one or more config values",
	RunE: func(cmd *cobra.Command, names []string) error {
		loader := configureLoader(cmd)
		provider, err := newProviderChecked(cmd.Context(), loader)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
		if err != nil {
			return err
		}

		if err := cli.ConfigDelete(cmd.Context(), projectName, provider, names...); err != nil {
			// Show a warning (not an error) if the config was not found
			if connect.CodeOf(err) == connect.CodeNotFound {
				term.Warn(cliClient.PrettyError(err))
				return nil
			}
			return err
		}
		term.Info("Deleted", names)

		printDefangHint("To list the configs (but not their values), do:", "config ls")
		return nil
	},
}

var configListCmd = &cobra.Command{
	Use:         "ls", // like Docker
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Aliases:     []string{"list"},
	Short:       "List configs",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		provider, err := newProviderChecked(cmd.Context(), loader)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
		if err != nil {
			return err
		}

		return cli.ConfigList(cmd.Context(), projectName, provider)
	},
}

var debugCmd = &cobra.Command{
	Use:         "debug [SERVICE...]",
	Annotations: authNeededAnnotation,
	Hidden:      true,
	Short:       "Debug a build, deployment, or service failure",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		etag, _ := cmd.Flags().GetString("etag")
		deployment, _ := cmd.Flags().GetString("deployment")
		since, _ := cmd.Flags().GetString("since")
		until, _ := cmd.Flags().GetString("until")

		if etag != "" && deployment == "" {
			deployment = etag
		}

		loader := configureLoader(cmd)
		_, err := newProviderChecked(ctx, loader)
		if err != nil {
			return err
		}

		project, err := loader.LoadProject(ctx)
		if err != nil {
			return err
		}

		debugger, err := debug.NewDebugger(ctx, global.Cluster, &global.Stack)
		if err != nil {
			return err
		}

		now := time.Now()
		sinceTs, err := timeutils.ParseTimeOrDuration(since, now)
		if err != nil {
			return fmt.Errorf("invalid 'since' time: %w", err)
		}
		untilTs, err := timeutils.ParseTimeOrDuration(until, now)
		if err != nil {
			return fmt.Errorf("invalid 'until' time: %w", err)
		}

		debugConfig := debug.DebugConfig{
			Deployment:     deployment,
			FailedServices: args,
			Project:        project,
			ProviderID:     &global.Stack.Provider,
			Since:          sinceTs.UTC(),
			Until:          untilTs.UTC(),
		}
		return debugger.DebugDeployment(ctx, debugConfig)
	},
}

var deleteCmd = &cobra.Command{
	Use:         "delete SERVICE...",
	Annotations: authNeededAnnotation,
	Args:        cobra.MinimumNArgs(1),
	Aliases:     []string{"del", "rm", "remove"},
	Hidden:      true,
	Short:       "Delete a service from the cluster",
	Deprecated:  "use 'compose down' instead",
	RunE: func(cmd *cobra.Command, names []string) error {
		var tail, _ = cmd.Flags().GetBool("tail")

		loader := configureLoader(cmd)
		provider, err := newProviderChecked(cmd.Context(), loader)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
		if err != nil {
			return err
		}

		err = canIUseProvider(cmd.Context(), provider, projectName, 0)
		if err != nil {
			return err
		}

		since := time.Now()
		deployment, err := cli.Delete(cmd.Context(), projectName, global.Client, provider, names...)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				// Show a warning (not an error) if the service was not found
				term.Warn(cliClient.PrettyError(err))
				return nil
			}
			return err
		}

		term.Info("Deleted service", names, "with deployment ID", deployment)

		if !tail {
			printDefangHint("To track the update, do:", "tail --deployment "+deployment)
			return nil
		}

		term.Info("Tailing logs for update; press Ctrl+C to detach:")

		tailOptions := cli.TailOptions{
			Deployment: deployment,
			LogType:    logs.LogTypeAll,
			Since:      since,
			Verbose:    global.Verbose,
		}
		tailCtx := cmd.Context() // FIXME: stop Tail when the deployment is done
		return cli.TailAndWaitForCD(tailCtx, provider, projectName, tailOptions)
	},
}

// deploymentsCmd and deploymentsListCmd do the same thing. deploymentsListCmd is for backward compatibility.
var deploymentsCmd = &cobra.Command{
	Use:         "deployments",
	Aliases:     []string{"deployment", "deploys", "deps", "dep"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Short:       "List active deployments across all projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		var projectName, _ = cmd.Flags().GetString("project-name")
		var utc, _ = cmd.Flags().GetBool("utc")

		if utc {
			cli.EnableUTCMode()
		}

		return cli.DeploymentsList(cmd.Context(), defangv1.DeploymentType_DEPLOYMENT_TYPE_ACTIVE, projectName, global.Client, 0)
	},
}

var deploymentsListCmd = &cobra.Command{
	Use:         "history",
	Aliases:     []string{"ls", "list"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Short:       "List deployment history for a project",
	RunE: func(cmd *cobra.Command, args []string) error {
		var utc, _ = cmd.Flags().GetBool("utc")

		if utc {
			cli.EnableUTCMode()
		}

		loader := configureLoader(cmd)
		projectName, err := loader.LoadProjectName(cmd.Context())
		if err != nil {
			return err
		}

		return cli.DeploymentsList(cmd.Context(), defangv1.DeploymentType_DEPLOYMENT_TYPE_HISTORY, projectName, global.Client, 10)
	},
}

var sendCmd = &cobra.Command{
	Use:         "send",
	Hidden:      true, // not available in private beta
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Aliases:     []string{"msg", "message", "publish", "pub"},
	Short:       "Send a message to a service",
	RunE: func(cmd *cobra.Command, args []string) error {
		var id, _ = cmd.Flags().GetString("id")
		var _type, _ = cmd.Flags().GetString("type")
		var data, _ = cmd.Flags().GetString("data")
		var contenttype, _ = cmd.Flags().GetString("content-type")
		var subject, _ = cmd.Flags().GetString("subject")

		return cli.SendMsg(cmd.Context(), global.Client, subject, _type, id, []byte(data), contenttype)
	},
}

var tokenCmd = &cobra.Command{
	Use:         "token",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Short:       "Manage personal access tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		var s, _ = cmd.Flags().GetString("scope")
		var expires, _ = cmd.Flags().GetDuration("expires")

		return cli.Token(cmd.Context(), global.Client, getTenantSelection(), expires, scope.Scope(s))
	},
}

var logoutCmd = &cobra.Command{
	Use:     "logout",
	Args:    cobra.NoArgs,
	Aliases: []string{"logoff", "revoke"},
	Short:   "Log out",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cli.Logout(cmd.Context(), global.Client); err != nil {
			return err
		}
		term.Info("Successfully logged out")
		return nil
	},
}

var tosCmd = &cobra.Command{
	Use:     "terms",
	Aliases: []string{"tos", "eula", "tac", "tou"},
	Args:    cobra.NoArgs,
	Short:   "Read and/or agree the Defang terms of service",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if we are correctly logged in
		if _, err := global.Client.WhoAmI(cmd.Context()); err != nil {
			return err
		}

		agree, _ := cmd.Flags().GetBool("agree-tos")

		if agree {
			return login.NonInteractiveAgreeToS(cmd.Context(), global.Client)
		}

		if global.NonInteractive {
			printDefangHint("To agree to the terms of service, do:", cmd.CalledAs()+" --agree-tos")
			return nil
		}

		return login.InteractiveAgreeToS(cmd.Context(), global.Client)
	},
}

var upgradeCmd = &cobra.Command{
	Use:     "upgrade",
	Args:    cobra.NoArgs,
	Aliases: []string{"update"},
	Short:   "Upgrade the Defang CLI to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		global.HideUpdate = true
		return cli.Upgrade(cmd.Context())
	},
}

func configureLoader(cmd *cobra.Command) *compose.Loader {
	configPaths, err := cmd.Flags().GetStringArray("file")
	if err != nil {
		panic(err)
	}

	projectName, err := cmd.Flags().GetString("project-name")
	if err != nil {
		panic(err)
	}

	// Avoid common mistakes
	var prov cliClient.ProviderID
	if prov.Set(projectName) == nil && !cmd.Flag("provider").Changed {
		// using -p with a provider name instead of -P
		term.Warnf("Project name %q looks like a provider name; did you mean to use -P=%s instead of -p?", projectName, projectName)
		doubleCheckProjectName(projectName)
	} else if strings.HasPrefix(projectName, "roject-name") {
		// -project-name= instead of --project-name
		term.Warn("Did you mean to use --project-name instead of -project-name?")
		doubleCheckProjectName(projectName)
	} else if strings.HasPrefix(projectName, "rovider") {
		// -provider= instead of --provider
		term.Warn("Did you mean to use --provider instead of -provider?")
		doubleCheckProjectName(projectName)
	}
	return compose.NewLoader(compose.WithProjectName(projectName), compose.WithPath(configPaths...))
}

func doubleCheckProjectName(projectName string) {
	if global.NonInteractive {
		return
	}
	var confirm bool
	err := survey.AskOne(&survey.Confirm{
		Message: "Continue with project: " + projectName + "?",
	}, &confirm, survey.WithStdio(term.DefaultTerm.Stdio()))
	track.Evt("ProjectNameConfirm", P("project", projectName), P("confirm", confirm), P("err", err))
	if err == nil && !confirm {
		os.Exit(1)
	}
}

func awsInEnv() bool {
	return os.Getenv("AWS_PROFILE") != "" || os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_SECRET_ACCESS_KEY") != ""
}

func doInEnv() bool {
	return os.Getenv("DIGITALOCEAN_ACCESS_TOKEN") != "" || os.Getenv("DIGITALOCEAN_TOKEN") != ""
}

func gcpInEnv() bool {
	return os.Getenv("GCP_PROJECT_ID") != "" || os.Getenv("CLOUDSDK_CORE_PROJECT") != ""
}

func awsInConfig(ctx context.Context) bool {
	_, err := aws.LoadDefaultConfig(ctx, aws.Region(""))
	return err == nil
}

func IsCompletionCommand(cmd *cobra.Command) bool {
	return cmd.Name() == cobra.ShellCompRequestCmd || (cmd.Parent() != nil && cmd.Parent().Name() == "completion")
}

func isUpgradeCommand(cmd *cobra.Command) bool {
	return cmd.Name() == "upgrade"
}

var providerDescription = map[cliClient.ProviderID]string{
	cliClient.ProviderDefang: "The Defang Playground is a free platform intended for testing purposes only.",
	cliClient.ProviderAWS:    "Deploy to AWS using the AWS_* environment variables or the AWS CLI configuration.",
	cliClient.ProviderDO:     "Deploy to DigitalOcean using the DIGITALOCEAN_TOKEN, SPACES_ACCESS_KEY_ID, and SPACES_SECRET_ACCESS_KEY environment variables.",
	cliClient.ProviderGCP:    "Deploy to Google Cloud Platform using gcloud Application Default Credentials.",
}

func getStack(ctx context.Context, ec elicitations.Controller, sm stacks.Manager) (*stacks.StackParameters, string, error) {
	stackSelector := stacks.NewSelector(ec, sm)

	var whence string
	stack := &stacks.StackParameters{
		Name:     "",
		Provider: cliClient.ProviderAuto,
		Mode:     modes.ModeUnspecified,
	}

	// This code unfortunately replicates the provider precedence rules in the
	// RoomCmd's PersistentPreRunE func, I think we should avoid reading the
	// stack file during startup, and only read it here instead.
	if os.Getenv("DEFANG_STACK") != "" || RootCmd.PersistentFlags().Changed("stack") {
		whence = "stack file"
		stackName := os.Getenv("DEFANG_STACK")
		if stackName == "" {
			stackName = RootCmd.Flags().Lookup("stack").Value.String()
		}
		stackParams, err := sm.Load(stackName)
		if err != nil {
			return nil, "", fmt.Errorf("unable to load stack %q: %w", stackName, err)
		}
		stack = stackParams

		if stack.Provider == cliClient.ProviderAuto {
			return nil, "", fmt.Errorf("stack %q has an invalid provider %q", stack.Name, stack.Provider)
		}
		return stack, whence, nil
	}

	knownStacks, err := sm.List(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("unable to list stacks: %w", err)
	}
	stackNames := make([]string, 0, len(knownStacks))
	for _, s := range knownStacks {
		stackNames = append(stackNames, s.Name)
	}
	if RootCmd.PersistentFlags().Changed("provider") {
		term.Warn("Warning: --provider flag is deprecated. Please use --stack instead. To learn about stacks, visit https://docs.defang.io/docs/concepts/stacks")
		providerIDString := RootCmd.Flags().Lookup("provider").Value.String()
		err := stack.Provider.Set(providerIDString)
		if err != nil {
			return nil, "", fmt.Errorf("invalid provider %q: %w", providerIDString, err)
		}
	} else if _, ok := os.LookupEnv("DEFANG_PROVIDER"); ok {
		term.Warn("Warning: DEFANG_PROVIDER environment variable is deprecated. Please use --stack instead. To learn about stacks, visit https://docs.defang.io/docs/concepts/stacks")
		providerIDString := os.Getenv("DEFANG_PROVIDER")
		err := stack.Provider.Set(providerIDString)
		if err != nil {
			return nil, "", fmt.Errorf("invalid provider %q: %w", providerIDString, err)
		}
	}
	if global.NonInteractive && stack.Provider == cliClient.ProviderAuto {
		whence = "non-interactive default"
		stack.Name = "beta"
		stack.Provider = cliClient.ProviderDefang
		return stack, whence, nil
	}

	// if there is exactly one stack with that provider, use it
	if len(knownStacks) == 1 && knownStacks[0].Provider == stack.Provider.String() {
		knownStack := knownStacks[0]
		// try to read the stackfile
		stack, loadErr := sm.Load(knownStack.Name)
		if loadErr != nil {
			term.Warn("unable to load stack from file, attempting to import from previous deployments", loadErr)
			importErr := importStack(sm, knownStack)
			if importErr != nil {
				return nil, "", fmt.Errorf("unable to load or import stack: %w", errors.Join(loadErr, importErr))
			}
		}

		whence = "only stack"
		return stack, whence, nil
	}

	// if there are zero known stacks or more than one known stack, prompt the user to create or select a stack
	if global.NonInteractive {
		if len(stackNames) > 0 {
			return nil, "", fmt.Errorf("please specify a stack using --stack. The following stacks are available: %v", stackNames)
		} else {
			return nil, "", fmt.Errorf("no stacks are configured; please create a stack using 'defang stack create --provider=%s'", stack.Provider)
		}
	}

	stackParameters, err := stackSelector.SelectStack(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to select stack: %w", err)
	}
	stack = stackParameters
	whence = "interactive selection"
	return stack, whence, nil
}

func importStack(sm stacks.Manager, stack stacks.StackListItem) error {
	var providerID cliClient.ProviderID
	err := providerID.Set(stack.Provider)
	if err != nil {
		return fmt.Errorf("invalid provider %q in stack %q: %w", stack.Provider, stack.Name, err)
	}
	mode := modes.ModeUnspecified
	if stack.Mode != "" {
		err = mode.Set(stack.Mode)
		if err != nil {
			return fmt.Errorf("invalid mode %q in stack %q: %w", stack.Mode, stack.Name, err)
		}
	}
	params := &stacks.StackParameters{
		Name:     stack.Name,
		Provider: providerID,
		Region:   stack.Region,
		Mode:     mode,
	}
	err = sm.LoadParameters(params.ToMap(), false)
	if err != nil {
		return fmt.Errorf("unable to load parameters for stack %q: %w", stack.Name, err)
	}

	return nil
}

func printProviderMismatchWarnings(ctx context.Context, provider cliClient.ProviderID) {
	if provider == cliClient.ProviderDefang {
		// Ignore any env vars when explicitly using the Defang playground provider
		// Defaults to defang provider in non-interactive mode
		if awsInEnv() {
			term.Warn("AWS environment variables were detected; did you forget --provider=aws or DEFANG_PROVIDER=aws?")
		}
		if doInEnv() {
			term.Warn("DIGITALOCEAN_TOKEN environment variable was detected; did you forget --provider=digitalocean or DEFANG_PROVIDER=digitalocean?")
		}
		if gcpInEnv() {
			term.Warn("GCP_PROJECT_ID/CLOUDSDK_CORE_PROJECT environment variable was detected; did you forget --provider=gcp or DEFANG_PROVIDER=gcp?")
		}
	}

	switch provider {
	case cliClient.ProviderAWS:
		if !awsInConfig(ctx) {
			term.Warn("AWS provider was selected, but AWS environment is not set")
		}
	case cliClient.ProviderDO:
		if !doInEnv() {
			term.Warn("DigitalOcean provider was selected, but DIGITALOCEAN_TOKEN environment variable is not set")
		}
	case cliClient.ProviderGCP:
		if !gcpInEnv() {
			term.Warn("GCP provider was selected, but GCP_PROJECT_ID environment variable is not set")
		}
	}
}

func newProvider(ctx context.Context, ec elicitations.Controller, sm stacks.Manager) (cliClient.Provider, error) {
	stack, whence, err := getStack(ctx, ec, sm)
	if err != nil {
		return nil, err
	}

	// TODO: avoid writing to this global variable once all readers are removed
	global.Stack = *stack

	extraMsg := ""
	if stack.Provider == cliClient.ProviderDefang {
		extraMsg = "; consider using BYOC (https://s.defang.io/byoc)"
	}
	term.Infof("Using the %q stack on %s from %s%s", stack.Name, stack.Provider, whence, extraMsg)

	printProviderMismatchWarnings(ctx, stack.Provider)
	provider := cli.NewProvider(ctx, stack.Provider, global.Client, stack.Name)
	return provider, nil
}

func newProviderChecked(ctx context.Context, loader cliClient.Loader) (cliClient.Provider, error) {
	var err error
	projectName := ""
	outside := true
	if loader != nil {
		projectName, err = loader.LoadProjectName(ctx)
		if err != nil {
			term.Warnf("Unable to load project: %v", err)
		}
		outside = loader.OutsideWorkingDirectory()
	}
	elicitationsClient := elicitations.NewSurveyClient(os.Stdin, os.Stdout, os.Stderr)
	ec := elicitations.NewController(elicitationsClient)
	var sm stacks.Manager
	if outside {
		sm, err = stacks.NewManager(global.Client, "", projectName)
		if err != nil {
			return nil, fmt.Errorf("failed to create stack manager: %w", err)
		}
	} else {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		sm, err = stacks.NewManager(global.Client, wd, projectName)
		if err != nil {
			return nil, fmt.Errorf("failed to create stack manager: %w", err)
		}
	}
	provider, err := newProvider(ctx, ec, sm)
	if err != nil {
		return nil, err
	}
	_, err = provider.AccountInfo(ctx)
	return provider, err
}

func canIUseProvider(ctx context.Context, provider cliClient.Provider, projectName string, serviceCount int) error {
	return cliClient.CanIUseProvider(ctx, global.Client, provider, projectName, serviceCount)
}
