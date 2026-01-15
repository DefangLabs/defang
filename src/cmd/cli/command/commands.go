package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/agent"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/gcp"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/github"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/mcp"
	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/bufbuild/connect-go"
	"github.com/spf13/cobra"
)

const authNeeded = "auth-needed" // annotation to indicate that a command needs authorization
var authNeededAnnotation = map[string]string{authNeeded: ""}

var P = track.P

var elicitationsClient = elicitations.NewSurveyClient(os.Stdin, os.Stdout, os.Stderr)
var ec = elicitations.NewController(elicitationsClient)

func Execute(ctx context.Context) error {
	if term.StdoutCanColor() {
		restore := term.EnableANSI()
		defer restore()
	}

	if err := RootCmd.ExecuteContext(ctx); err != nil {
		if !(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			term.Error("Error:", client.PrettyError(err))
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
func SetupCommands(version string) {
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
	RootCmd.PersistentFlags().Var(&global.Tenant, "workspace", "workspace to use")
	RootCmd.PersistentFlags().VarP(&global.Stack.Provider, "provider", "P", fmt.Sprintf(`bring-your-own-cloud provider; one of %v`, client.AllProviders()))
	RootCmd.RegisterFlagCompletionFunc("provider", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var completions []cobra.Completion
		for _, provider := range client.AllProviders() {
			completions = append(completions, provider.String())
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	})
	// RootCmd.Flag("provider").NoOptDefVal = "auto" NO this will break the "--provider aws"
	RootCmd.Flags().MarkDeprecated("provider", "use '--stack' to select a stack instead")
	RootCmd.PersistentFlags().BoolVarP(&global.Verbose, "verbose", "v", global.Verbose, "verbose logging") // backwards compat: only used by tail
	RootCmd.PersistentFlags().BoolVar(&global.Debug, "debug", global.Debug, "debug logging for troubleshooting the CLI")
	RootCmd.PersistentFlags().BoolVar(&dryrun.DoDryRun, "dry-run", false, "dry run (don't actually change anything)")
	RootCmd.PersistentFlags().BoolVar(&global.NonInteractive, "non-interactive", global.NonInteractive, "disable interactive prompts / no TTY")
	RootCmd.PersistentFlags().StringP("project-name", "p", "", "project name")
	RootCmd.PersistentFlags().StringP("cwd", "C", "", "change directory before running the command")
	_ = RootCmd.MarkPersistentFlagDirname("cwd")
	RootCmd.PersistentFlags().StringArrayP("file", "f", []string{}, `compose file path(s)`)
	_ = RootCmd.MarkPersistentFlagFilename("file", "yml", "yaml")

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
	cdCmd.AddCommand(cdOutputsCmd)
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

	// Init command
	sourcePlatform := migrate.SourcePlatformUnspecified
	initCmd.PersistentFlags().Var(&sourcePlatform, "from", fmt.Sprintf(`the platform from which to migrate the project; one of %v`, migrate.AllSourcePlatforms))
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

	configCmd.AddCommand(configResolveCmd)

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
	debugCmd.Flags().String("since", "", "start time for logs; duration or timestamp (unix or RFC3339)")
	debugCmd.Flags().String("until", "", "end time for logs; duration or timestamp (unix or RFC3339)")
	debugCmd.Flags().StringVar(&global.ModelID, "model", global.ModelID, "LLM model to use for debugging (Pro users only)")
	RootCmd.AddCommand(debugCmd)

	// Tail Command
	tailCmd := makeTailCmd()
	RootCmd.AddCommand(tailCmd)

	// Logs Command
	logsCmd := makeLogsCmd()
	RootCmd.AddCommand(logsCmd)

	// Deployments Command
	deploymentsCmd.AddCommand(deploymentsListCmd)
	deploymentsCmd.PersistentFlags().Bool("utc", false, "show logs in UTC timezone (ie. TZ=UTC)")
	deploymentsCmd.PersistentFlags().Uint32P("limit", "l", 10, "maximum number of deployments to list")
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
		if isCompletionCommand(cmd) {
			// but do change the directory for file completions to work correctly.
			// Unfortunately, Cobra will not have parsed the "cwd" flag.
			if cwd := getCwd(os.Args); cwd != "" {
				return os.Chdir(cwd)
			}
			return nil
		}

		// Create a temporary gRPC client for tracking events before login
		track.Tracker = cli.Connect(global.Cluster, global.Tenant)

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

		global.Client, err = cli.ConnectWithTenant(ctx, global.Cluster, global.Tenant)
		if err != nil {
			if connect.CodeOf(err) != connect.CodeUnauthenticated {
				return err
			}
			term.Debug("Using existing token failed; continuing to allow login/ToS flow:", err)
		}

		track.Tracker = global.Client // update tracker with the real client

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

func configureLoader(cmd *cobra.Command) *compose.Loader {
	loaderFlags := newSessionLoaderOptionsForCommand(cmd)
	return compose.NewLoader(compose.WithProjectName(loaderFlags.ProjectName), compose.WithPath(loaderFlags.ComposeFilePaths...))
}

func isCompletionCommand(cmd *cobra.Command) bool {
	return cmd.Name() == cobra.ShellCompRequestCmd || (cmd.Parent() != nil && cmd.Parent().Name() == "completion")
}

func isUpgradeCommand(cmd *cobra.Command) bool {
	return cmd.Name() == "upgrade"
}

func canIUseProvider(ctx context.Context, provider client.Provider, projectName string, serviceCount int) error {
	return client.CanIUseProvider(ctx, global.Client, provider, projectName, serviceCount)
}
