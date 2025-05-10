package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/spf13/cobra"
)

const authNeeded = "auth-needed" // annotation to indicate that a command needs authorization
var authNeededAnnotation = map[string]string{authNeeded: ""}

var P = track.P

// GLOBALS
var (
	client         *cliClient.GrpcClient
	cluster        string
	colorMode      = ColorAuto
	doDebug        = false
	hasTty         = term.IsTerminal() && !pkg.GetenvBool("CI")
	hideUpdate     = pkg.GetenvBool("DEFANG_HIDE_UPDATE")
	mode           = Mode(defangv1.DeploymentMode_MODE_UNSPECIFIED)
	modelId        = os.Getenv("DEFANG_MODEL_ID") // for Pro users only
	nonInteractive = !hasTty
	org            string
	providerID     = cliClient.ProviderID(pkg.Getenv("DEFANG_PROVIDER", "auto"))
	verbose        = false
)

func getCluster() string {
	if org == "" {
		return cluster
	}
	return org + "@" + cluster
}

func prettyError(err error) error {
	// To avoid printing the internal gRPC error code
	var cerr *connect.Error
	if errors.As(err, &cerr) {
		term.Debug("Server error:", cerr)
		err = errors.Unwrap(cerr)
	}
	return err
}

func Execute(ctx context.Context) error {
	if term.StdoutCanColor() {
		restore := term.EnableANSI()
		defer restore()
	}

	if err := RootCmd.ExecuteContext(ctx); err != nil {
		if !(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			term.Error("Error:", prettyError(err))
		}

		if err == cli.ErrDryRun {
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
			provider, err := getProvider(ctx, nil)
			if err != nil {
				return err
			}
			if resp, err := provider.GetServices(ctx, &defangv1.GetServicesRequest{Project: projectName}); err == nil {
				projectName = resp.Project
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
		return ExitCode(code)
	}

	if hasTty && term.HadWarnings() {
		fmt.Println("For help with warnings, check our FAQ at https://s.defang.io/warnings")
	}

	if hasTty && !hideUpdate && rand.Intn(10) == 0 {
		if latest, err := GetLatestVersion(ctx); err == nil && isNewer(GetCurrentVersion(), latest) {
			term.Debug("Latest Version:", latest, "Current Version:", GetCurrentVersion())
			fmt.Println("A newer version of the CLI is available at https://github.com/DefangLabs/defang/releases/latest")
			if rand.Intn(10) == 0 && !pkg.GetenvBool("DEFANG_HIDE_HINTS") {
				fmt.Println("To silence these notices, do: export DEFANG_HIDE_UPDATE=1")
			}
		}
	}

	return nil
}

func SetupCommands(ctx context.Context, version string) {
	cobra.EnableTraverseRunHooks = true // we always need to run the RootCmd's pre-run hook

	RootCmd.Version = version
	RootCmd.PersistentFlags().Var(&colorMode, "color", fmt.Sprintf(`colorize output; one of %v`, allColorModes))
	RootCmd.PersistentFlags().StringVarP(&cluster, "cluster", "s", cli.DefangFabric, "Defang cluster to connect to")
	RootCmd.PersistentFlags().MarkHidden("cluster")
	RootCmd.PersistentFlags().StringVar(&org, "org", os.Getenv("DEFANG_ORG"), "override GitHub organization name (tenant)")
	RootCmd.PersistentFlags().VarP(&providerID, "provider", "P", fmt.Sprintf(`bring-your-own-cloud provider; one of %v`, cliClient.AllProviders()))
	// RootCmd.Flag("provider").NoOptDefVal = "auto" NO this will break the "--provider aws"
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose logging") // backwards compat: only used by tail
	RootCmd.PersistentFlags().BoolVar(&doDebug, "debug", pkg.GetenvBool("DEFANG_DEBUG"), "debug logging for troubleshooting the CLI")
	RootCmd.PersistentFlags().BoolVar(&cli.DoDryRun, "dry-run", false, "dry run (don't actually change anything)")
	RootCmd.PersistentFlags().BoolVarP(&nonInteractive, "non-interactive", "T", !hasTty, "disable interactive prompts / no TTY")
	RootCmd.PersistentFlags().StringP("project-name", "p", "", "project name")
	RootCmd.PersistentFlags().StringP("cwd", "C", "", "change directory before running the command")
	_ = RootCmd.MarkPersistentFlagDirname("cwd")
	RootCmd.PersistentFlags().StringArrayP("file", "f", []string{}, `compose file path(s)`)
	_ = RootCmd.MarkPersistentFlagFilename("file", "yml", "yaml")

	// Create a temporary gRPC client for tracking events before login
	cli.Connect(ctx, cluster)

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
	cdListCmd.Flags().Bool("remote", false, "invoke the command on the remote cluster")
	cdCmd.AddCommand(cdListCmd)
	cdCmd.AddCommand(cdCancelCmd)
	cdPreviewCmd.Flags().VarP(&mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", allModes()))
	cdCmd.AddCommand(cdPreviewCmd)

	// Eula command
	tosCmd.Flags().Bool("agree-tos", false, "agree to the Defang terms of service")
	RootCmd.AddCommand(tosCmd)

	// Upgrade command
	RootCmd.AddCommand(upgradeCmd)

	// Token command
	tokenCmd.Flags().Duration("expires", 24*time.Hour, "validity duration of the token")
	tokenCmd.Flags().String("scope", "", fmt.Sprintf("scope of the token; one of %v (required)", scope.All())) // TODO: make it an Option
	_ = tokenCmd.MarkFlagRequired("scope")
	RootCmd.AddCommand(tokenCmd)

	// Login Command
	loginCmd.Flags().Bool("training-opt-out", false, "Opt out of ML training (Pro users only)")
	// loginCmd.Flags().Bool("skip-prompt", false, "skip the login prompt if already logged in"); TODO: Implement this
	RootCmd.AddCommand(loginCmd)

	// Whoami Command
	RootCmd.AddCommand(whoamiCmd)

	// Logout Command
	RootCmd.AddCommand(logoutCmd)

	// Generate Command
	generateCmd.Flags().StringVar(&modelId, "model", modelId, "LLM model to use for generating the code (Pro users only)")
	RootCmd.AddCommand(generateCmd)
	RootCmd.AddCommand(newCmd)

	// Get Services Command
	lsCommand := makeComposePsCmd()
	lsCommand.Use = "services"
	// TODO: when we add multi-project support to the playground, differentiate
	// between ls and ps
	lsCommand.Aliases = []string{"getServices", "ps", "ls", "list"}
	RootCmd.AddCommand(lsCommand)

	// Get Status Command
	RootCmd.AddCommand(getVersionCmd)

	// Config Command (was: secrets)
	configSetCmd.Flags().BoolP("name", "n", false, "name of the config (backwards compat)")
	configSetCmd.Flags().BoolP("env", "e", false, "set the config from an environment variable")
	configSetCmd.Flags().Bool("random", false, "set a secure randomly generated value for config")
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

	// Debug Command
	debugCmd.Flags().String("etag", "", "deployment ID (ETag) of the service")
	debugCmd.Flags().MarkHidden("etag")
	debugCmd.Flags().String("deployment", "", "deployment ID of the service")
	debugCmd.Flags().String("since", "", "start time for logs (RFC3339 format)")
	debugCmd.Flags().String("until", "", "end time for logs (RFC3339 format)")
	debugCmd.Flags().StringVar(&modelId, "model", modelId, "LLM model to use for debugging (Pro users only)")
	RootCmd.AddCommand(debugCmd)

	// Tail Command
	tailCmd := makeComposeLogsCmd()
	tailCmd.Use = "tail [SERVICE...]"
	tailCmd.Aliases = []string{"logs"}
	RootCmd.AddCommand(tailCmd)

	// Delete Command
	deleteCmd.Flags().BoolP("name", "n", false, "name of the service(s) (backwards compat)")
	_ = deleteCmd.Flags().MarkHidden("name")
	deleteCmd.Flags().Bool("tail", false, "tail the service logs after deleting")
	RootCmd.AddCommand(deleteCmd)

	// Deployments Command
	deploymentsCmd.AddCommand(deploymentsListCmd)
	RootCmd.AddCommand(deploymentsCmd)

	// MCP Command
	mcpCmd.AddCommand(mcpSetupCmd)
	mcpCmd.AddCommand(mcpServerCmd)
	mcpSetupCmd.Flags().String("client", "", "MCP setup client (supports: claude, windsurf, cursor, vscode)")
	mcpSetupCmd.MarkFlagRequired("client")
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

	if term.StdoutCanColor() { // TODO: should use DoColor(…) instead
		// Add some emphasis to the help command
		re := regexp.MustCompile(`(?m)^[A-Za-z ]+?:`)
		templ := re.ReplaceAllString(RootCmd.UsageTemplate(), "\033[1m$0\033[0m")
		RootCmd.SetUsageTemplate(templ)
	}

	origHelpFunc := RootCmd.HelpFunc()
	RootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		track.Cmd(cmd, "Help", P("args", args))
		origHelpFunc(cmd, args)
	})
}

var RootCmd = &cobra.Command{
	SilenceUsage:  true,
	SilenceErrors: true,
	Use:           "defang",
	Args:          cobra.NoArgs,
	Short:         "Defang CLI is used to take your app from Docker Compose to a secure and scalable deployment on your favorite cloud in minutes.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		term.SetDebug(doDebug)

		// Don't track/connect the completion commands
		if IsCompletionCommand(cmd) {
			return nil
		}

		// Use "defer" to track any errors that occur during the command
		defer func() {
			track.Cmd(cmd, "Invoked", P("args", args), P("err", err), P("non-interactive", nonInteractive), P("provider", providerID))
		}()

		// Do this first, since any errors will be printed to the console
		switch colorMode {
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

		client, err = cli.Connect(cmd.Context(), getCluster())
		if cli.IsNetworkError(err) {
			return fmt.Errorf("unable to connect to Defang server %q; please check network settings and try again", cluster)
		}

		if v, err := client.GetVersions(cmd.Context()); err == nil {
			version := cmd.Root().Version // HACK to avoid circular dependency with RootCmd
			term.Debug("Fabric:", v.Fabric, "CLI:", version, "CLI-Min:", v.CliMin)
			if hasTty && isNewer(version, v.CliMin) && !isUpgradeCommand(cmd) {
				term.Warn("Your CLI version is outdated. Please upgrade to the latest version by running:\n\n  defang upgrade\n")
				hideUpdate = true // hide the upgrade hint at the end
			}
		}

		// Check if we are correctly logged in, but only if the command needs authorization
		if _, ok := cmd.Annotations[authNeeded]; !ok {
			return nil
		}

		if err = client.CheckLoginAndToS(cmd.Context()); err != nil {
			if nonInteractive {
				return err
			}
			// Login interactively now; only do this for authorization-related errors
			if connect.CodeOf(err) == connect.CodeUnauthenticated {
				term.Debug("Server error:", err)
				term.Warn("Please log in to continue.")
				term.ResetWarnings() // clear any previous warnings so we don't show them again

				defer func() { track.Cmd(nil, "Login", P("reason", err)) }()
				if err = cli.InteractiveLogin(cmd.Context(), client, getCluster()); err != nil {
					return err
				}

				// Reconnect with the new token
				if client, err = cli.Connect(cmd.Context(), getCluster()); err != nil {
					return err
				}

				if err = client.CheckLoginAndToS(cmd.Context()); err == nil { // recheck (new token = new user)
					return nil // success
				}
			}

			// Check if the user has agreed to the terms of service and show a prompt if needed
			if connect.CodeOf(err) == connect.CodeFailedPrecondition {
				term.Warn(prettyError(err))

				defer func() { track.Cmd(nil, "Terms", P("reason", err)) }()
				if err = cli.InteractiveAgreeToS(cmd.Context(), client); err != nil {
					return err // fatal
				}
			}
		}

		return err
	},
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Args:  cobra.NoArgs,
	Short: "Authenticate to Defang",
	RunE: func(cmd *cobra.Command, args []string) error {
		trainingOptOut, _ := cmd.Flags().GetBool("training-opt-out")

		if nonInteractive {
			if err := cli.NonInteractiveLogin(cmd.Context(), client, getCluster()); err != nil {
				return err
			}
		} else {
			err := cli.InteractiveLogin(cmd.Context(), client, getCluster())
			if err != nil {
				return err
			}

			printDefangHint("To generate a sample service, do:", "generate")
		}

		if trainingOptOut {
			req := &defangv1.SetOptionsRequest{TrainingOptOut: trainingOptOut}
			if err := client.SetOptions(cmd.Context(), req); err != nil {
				return err
			}
			term.Info("Options updated successfully")
		}
		return nil
	},
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Args:  cobra.NoArgs,
	Short: "Show the current user",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		nonInteractive = true // don't show provider prompt
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			term.Debug("unable to get provider:", err)
		}

		str, err := cli.Whoami(cmd.Context(), client, provider)
		if err != nil {
			return err
		}

		term.Info(str)
		return nil
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

		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		if err := cli.GenerateLetsEncryptCert(cmd.Context(), project, client, provider); err != nil {
			return err
		}
		return nil
	},
}

var generateCmd = &cobra.Command{
	Use:     "generate",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"gen"},
	Short:   "Generate a sample Defang project",
	RunE: func(cmd *cobra.Command, args []string) error {
		var sample, language, defaultFolder string
		if len(args) > 0 {
			sample = args[0]
		}

		if nonInteractive {
			if sample == "" {
				return errors.New("cannot run in non-interactive mode")
			}
			return cli.InitFromSamples(cmd.Context(), "", []string{sample})
		}

		sampleList, fetchSamplesErr := cli.FetchSamples(cmd.Context())
		if sample == "" {
			// Fetch the list of samples from the Defang repository
			if fetchSamplesErr != nil {
				term.Debug("unable to fetch samples:", fetchSamplesErr)
			} else if len(sampleList) > 0 {
				const generateWithAI = "Generate with AI"

				sampleNames := []string{generateWithAI}
				sampleTitles := []string{"Generate a sample from scratch using a language prompt"}
				sampleIndex := []string{"unused first entry because we always show genAI option"}
				for _, sample := range sampleList {
					sampleNames = append(sampleNames, sample.Name)
					sampleTitles = append(sampleTitles, sample.Title)
					sampleIndex = append(sampleIndex, strings.ToLower(sample.Name+" "+sample.Title+" "+
						strings.Join(sample.Tags, " ")+" "+strings.Join(sample.Languages, " ")))
				}

				if err := survey.AskOne(&survey.Select{
					Message: "Choose a sample service:",
					Options: sampleNames,
					Help:    "The project code will be based on the sample you choose here.",
					Filter: func(filter string, value string, i int) bool {
						return i == 0 || strings.Contains(sampleIndex[i], strings.ToLower(filter))
					},
					Description: func(value string, i int) string {
						return sampleTitles[i]
					},
				}, &sample, survey.WithStdio(term.DefaultTerm.Stdio())); err != nil {
					return err
				}
				if sample == generateWithAI {
					if err := survey.AskOne(&survey.Select{
						Message: "Choose the language you'd like to use:",
						Options: cli.SupportedLanguages,
						Help:    "The project code will be in the language you choose here.",
					}, &language, survey.WithStdio(term.DefaultTerm.Stdio())); err != nil {
						return err
					}
					sample = ""
					defaultFolder = "project1"
				} else {
					defaultFolder = sample
				}
			}
		}

		var qs = []*survey.Question{
			{
				Name: "description",
				Prompt: &survey.Input{
					Message: "Please describe the service you'd like to build:",
					Help: `Here are some example prompts you can use:
    "A simple 'hello world' function"
    "A service with 2 endpoints, one to upload and the other to download a file from AWS S3"
    "A service with a default endpoint that returns an HTML page with a form asking for the user's name and then a POST endpoint to handle the form post when the user clicks the 'submit' button"`,
				},
				Validate: survey.MinLength(5),
			},
			{
				Name: "folder",
				Prompt: &survey.Input{
					Message: "What folder would you like to create the project in?",
					Default: defaultFolder, // dynamically set based on chosen sample
					Help:    "The generated code will be in the folder you choose here. If the folder does not exist, it will be created.",
				},
				Validate: survey.Required,
			},
		}

		if sample != "" {
			qs = qs[1:] // user picked a sample, so we skip the description question
			sampleExists := slices.ContainsFunc(sampleList, func(s cli.Sample) bool {
				return s.Name == sample
			})

			if !sampleExists {
				return cli.ErrSampleNotFound
			}
		}

		prompt := struct {
			Description string // or you can tag fields to match a specific name
			Folder      string
		}{}

		// ask the remaining questions
		err := survey.Ask(qs, &prompt, survey.WithStdio(term.DefaultTerm.Stdio()))
		if err != nil {
			return err
		}

		if client.CheckLoginAndToS(cmd.Context()) != nil {
			// The user is either not logged in or has not agreed to the terms of service; ask for agreement to the terms now
			if err := cli.InteractiveAgreeToS(cmd.Context(), client); err != nil {
				// This might fail because the user did not log in. This is fine: server won't save the terms agreement, but can proceed with the generation
				if connect.CodeOf(err) != connect.CodeUnauthenticated {
					return err
				}
			}
		}

		track.Evt("Generate Started", P("language", language), P("sample", sample), P("description", prompt.Description), P("folder", prompt.Folder), P("model", modelId))

		// Check if the current folder is empty
		if empty, err := pkg.IsDirEmpty(prompt.Folder); !os.IsNotExist(err) && !empty {
			nonEmptyFolder := fmt.Sprintf("The folder %q is not empty. We recommend running this command in an empty folder.", prompt.Folder)

			var confirm bool
			err := survey.AskOne(&survey.Confirm{
				Message: nonEmptyFolder + " Continue creating project?",
			}, &confirm, survey.WithStdio(term.DefaultTerm.Stdio()))
			if err == nil && !confirm {
				os.Exit(1)
			}
		}

		if sample != "" {
			term.Info("Fetching sample from the Defang repository...")
			err := cli.InitFromSamples(cmd.Context(), prompt.Folder, []string{sample})
			if err != nil {
				return err
			}
		} else {
			term.Info("Working on it. This may take 1 or 2 minutes...")
			args := cli.GenerateArgs{
				Description: prompt.Description,
				Folder:      prompt.Folder,
				Language:    language,
				ModelId:     modelId,
			}
			_, err := cli.GenerateWithAI(cmd.Context(), client, args)
			if err != nil {
				return err
			}
		}

		term.Info("Code generated successfully in folder", prompt.Folder)

		editor := pkg.Getenv("DEFANG_EDITOR", "code") // TODO: should we use EDITOR env var instead?
		cmdd := exec.Command(editor, prompt.Folder)
		err = cmdd.Start()
		if err != nil {
			term.Debugf("unable to launch editor %q: %v", editor, err)
		}

		cd := ""
		if prompt.Folder != "." {
			cd = "`cd " + prompt.Folder + "` and "
		}

		// Load the project and check for empty environment variables
		loader := compose.NewLoader(compose.WithPath(filepath.Join(prompt.Folder, "compose.yaml")))
		project, err := loader.LoadProject(cmd.Context())
		if err != nil {
			term.Debugf("unable to load new project: %v", err)
		}

		var envInstructions []string
		for _, envVar := range collectUnsetEnvVars(project) {
			envInstructions = append(envInstructions, "config create "+envVar)
		}

		if len(envInstructions) > 0 {
			printDefangHint("Check the files in your favorite editor.\nTo configure the service, do "+cd, envInstructions...)
		} else {
			printDefangHint("Check the files in your favorite editor.\nTo deploy the service, do "+cd, "compose up")
		}

		return nil
	},
}

var newCmd = &cobra.Command{
	Use:     "new [SAMPLE]",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"init"},
	Short:   "Create a new Defang project from a sample",
	RunE:    generateCmd.RunE,
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

var getVersionCmd = &cobra.Command{
	Use:     "version",
	Args:    cobra.NoArgs,
	Aliases: []string{"ver", "stat", "status"}, // for backwards compatibility
	Short:   "Get version information for the CLI and Fabric service",
	RunE: func(cmd *cobra.Command, args []string) error {
		term.Printc(term.BrightCyan, "Defang CLI:    ")
		fmt.Println(GetCurrentVersion())

		term.Printc(term.BrightCyan, "Latest CLI:    ")
		ver, err := GetLatestVersion(cmd.Context())
		fmt.Println(ver)

		term.Printc(term.BrightCyan, "Defang Fabric: ")
		ver, err2 := cli.GetVersion(cmd.Context(), client)
		fmt.Println(ver)
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
	Args:        cobra.RangeArgs(1, 2),
	Aliases:     []string{"set", "add", "put"},
	Short:       "Adds or updates a sensitive config value",
	RunE: func(cmd *cobra.Command, args []string) error {
		fromEnv, _ := cmd.Flags().GetBool("env")
		random, _ := cmd.Flags().GetBool("random")

		// Make sure we have a project to set config for before asking for a value
		loader := configureLoader(cmd)
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
		if err != nil {
			return err
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
		} else if nonInteractive || len(args) == 2 {
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
			value = CreateRandomConfigValue()
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
		provider, err := getProvider(cmd.Context(), loader)
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
				term.Warn(prettyError(err))
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
		provider, err := getProvider(cmd.Context(), loader)
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
		etag, _ := cmd.Flags().GetString("etag")
		deployment, _ := cmd.Flags().GetString("deployment")
		since, _ := cmd.Flags().GetString("since")
		until, _ := cmd.Flags().GetString("until")

		if etag != "" && deployment == "" {
			deployment = etag
		}

		loader := configureLoader(cmd)
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		project, err := loader.LoadProject(cmd.Context())
		if err != nil {
			return err
		}

		now := time.Now()
		sinceTs, err := cli.ParseTimeOrDuration(since, now)
		if err != nil {
			return fmt.Errorf("invalid 'since' time: %w", err)
		}
		untilTs, err := cli.ParseTimeOrDuration(until, now)
		if err != nil {
			return fmt.Errorf("invalid 'until' time: %w", err)
		}

		debugConfig := cli.DebugConfig{
			Deployment:     deployment,
			FailedServices: args,
			ModelId:        modelId,
			Project:        project,
			Provider:       provider,
			Since:          sinceTs.UTC(),
			Until:          untilTs.UTC(),
		}
		return cli.DebugDeployment(cmd.Context(), client, debugConfig)
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
		provider, err := getProvider(cmd.Context(), loader)
		if err != nil {
			return err
		}

		projectName, err := cliClient.LoadProjectNameWithFallback(cmd.Context(), loader, provider)
		if err != nil {
			return err
		}

		err = canIUseProvider(cmd.Context(), provider, projectName)
		if err != nil {
			return err
		}

		since := time.Now()
		deployment, err := cli.Delete(cmd.Context(), projectName, client, provider, names...)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				// Show a warning (not an error) if the service was not found
				term.Warn(prettyError(err))
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
			Verbose:    verbose,
		}
		tailCtx := cmd.Context() // FIXME: stop Tail when the deployment is done
		return cli.Tail(tailCtx, provider, projectName, tailOptions)
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
		return cli.DeploymentsList(cmd.Context(), defangv1.DeploymentType_DEPLOYMENT_TYPE_ACTIVE, "", *client, 0)
	},
}

var deploymentsListCmd = &cobra.Command{
	Use:         "list",
	Aliases:     []string{"ls"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Short:       "List deployment history for a project",
	RunE: func(cmd *cobra.Command, args []string) error {
		loader := configureLoader(cmd)
		projectName, err := loader.LoadProjectName(cmd.Context())
		if err != nil {
			return err
		}

		return cli.DeploymentsList(cmd.Context(), defangv1.DeploymentType_DEPLOYMENT_TYPE_HISTORY, projectName, *client, 10)
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

		return cli.SendMsg(cmd.Context(), client, subject, _type, id, []byte(data), contenttype)
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

		// TODO: should default to use the current tenant, not the default tenant
		return cli.Token(cmd.Context(), client, types.DEFAULT_TENANT, expires, scope.Scope(s))
	},
}

var logoutCmd = &cobra.Command{
	Use:     "logout",
	Args:    cobra.NoArgs,
	Aliases: []string{"logoff", "revoke"},
	Short:   "Log out",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cli.Logout(cmd.Context(), client); err != nil {
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
		if _, err := client.WhoAmI(cmd.Context()); err != nil {
			return err
		}

		agree, _ := cmd.Flags().GetBool("agree-tos")

		if agree {
			return cli.NonInteractiveAgreeToS(cmd.Context(), client)
		}

		if !nonInteractive {
			return cli.InteractiveAgreeToS(cmd.Context(), client)
		}

		printDefangHint("To agree to the terms of service, do:", cmd.CalledAs()+" --agree-tos")
		return nil
	},
}

var upgradeCmd = &cobra.Command{
	Use:     "upgrade",
	Args:    cobra.NoArgs,
	Aliases: []string{"update"},
	Short:   "Upgrade the Defang CLI to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		hideUpdate = true
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
		doubleCheck(projectName)
	} else if strings.HasPrefix(projectName, "roject-name") {
		// -project-name= instead of --project-name
		term.Warn("Did you mean to use --project-name instead of -project-name?")
		doubleCheck(projectName)
	} else if strings.HasPrefix(projectName, "rovider") {
		// -provider= instead of --provider
		term.Warn("Did you mean to use --provider instead of -provider?")
		doubleCheck(projectName)
	}
	return compose.NewLoader(compose.WithProjectName(projectName), compose.WithPath(configPaths...))
}

func doubleCheck(projectName string) {
	if !nonInteractive {
		var confirm bool
		err := survey.AskOne(&survey.Confirm{
			Message: "Continue with project: " + projectName + "?",
		}, &confirm, survey.WithStdio(term.DefaultTerm.Stdio()))
		track.Evt("ProjectNameConfirm", P("project", projectName), P("confirm", confirm), P("err", err))
		if err == nil && !confirm {
			os.Exit(1)
		}
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

func getProvider(ctx context.Context, loader cliClient.Loader) (cliClient.Provider, error) {
	extraMsg := ""
	source := "default project"

	// Command line flag takes precedence over environment variable
	if RootCmd.PersistentFlags().Changed("provider") {
		source = "command line flag"
	} else if val, ok := os.LookupEnv("DEFANG_PROVIDER"); ok {
		// Sanitize the provider value from the environment variable
		if err := providerID.Set(val); err != nil {
			return nil, fmt.Errorf("invalid provider '%v' in environment variable DEFANG_PROVIDER, supported providers are: %v", val, cliClient.AllProviders())
		}
		source = "environment variable"
	}

	switch providerID {
	case cliClient.ProviderAuto:
		if !nonInteractive {
			var err error
			if source, err = determineProviderID(ctx, loader); err != nil {
				return nil, err
			}
		} else {
			// Defaults to defang provider in non-interactive mode
			if awsInEnv() {
				term.Warn("Using Defang playground, but AWS environment variables were detected; did you forget --provider=aws or DEFANG_PROVIDER=aws?")
			}
			if doInEnv() {
				term.Warn("Using Defang playground, but DIGITALOCEAN_TOKEN environment variable was detected; did you forget --provider=digitalocean or DEFANG_PROVIDER=digitalocean?")
			}
			if gcpInEnv() {
				term.Warn("Using Defang playground, but GCP_PROJECT_ID/CLOUDSDK_CORE_PROJECT environment variable was detected; did you forget --provider=gcp or DEFANG_PROVIDER=gcp?")
			}
			providerID = cliClient.ProviderDefang
		}
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
	case cliClient.ProviderDefang:
		// Ignore any env vars when explicitly using the Defang playground provider
		extraMsg = "; consider using BYOC (https://s.defang.io/byoc)"
	}

	term.Infof("Using %s provider from %s%s", providerID.Name(), source, extraMsg)
	provider, err := cli.NewProvider(ctx, providerID, client)
	if err != nil {
		return nil, err
	}

	return provider, nil
}

func canIUseProvider(ctx context.Context, provider cliClient.Provider, projectName string) error {
	canUseReq := defangv1.CanIUseRequest{
		Project:  projectName,
		Provider: providerID.EnumValue(),
	}

	resp, err := client.CanIUse(ctx, &canUseReq)
	if err != nil {
		return err
	}
	provider.SetCanIUseConfig(resp)
	return nil
}

func determineProviderID(ctx context.Context, loader cliClient.Loader) (string, error) {
	var projectName string
	if loader != nil {
		var err error
		projectName, err = loader.LoadProjectName(ctx)
		if err != nil {
			term.Warnf("Unable to load project: %v", err)
		}

		if projectName != "" && !RootCmd.PersistentFlags().Changed("provider") { // If user manually selected auto provider, do not load from remote
			resp, err := client.GetSelectedProvider(ctx, &defangv1.GetSelectedProviderRequest{Project: projectName})
			if err != nil {
				term.Warnf("Unable to get selected provider: %v", err)
			} else if resp.Provider != defangv1.Provider_PROVIDER_UNSPECIFIED {
				providerID.SetEnumValue(resp.Provider)
				return "stored preference", nil
			}
		}
	}

	// Prompt the user to choose a provider if in interactive mode
	options := []string{}
	for _, p := range cliClient.AllProviders() {
		options = append(options, p.String())
	}
	// Default to the provider in the environment if available
	var defaultOption any // not string!
	if awsInEnv() {
		defaultOption = cliClient.ProviderAWS.String()
	} else if doInEnv() {
		defaultOption = cliClient.ProviderDO.String()
	} else if gcpInEnv() {
		defaultOption = cliClient.ProviderGCP.String()
	}
	var optionValue string
	if err := survey.AskOne(&survey.Select{
		Default: defaultOption,
		Message: "Choose a cloud provider:",
		Options: options,
		Help:    "The provider you choose will be used for deploying services.",
		Description: func(value string, i int) string {
			return providerDescription[cliClient.ProviderID(value)]
		},
	}, &optionValue, survey.WithStdio(term.DefaultTerm.Stdio())); err != nil {
		return "", err
	}
	track.Evt("ProviderSelected", P("provider", optionValue))
	if err := providerID.Set(optionValue); err != nil {
		panic(err)
	}

	// Save the selected provider to the fabric
	if projectName != "" {
		if err := client.SetSelectedProvider(ctx, &defangv1.SetSelectedProviderRequest{Project: projectName, Provider: providerID.EnumValue()}); err != nil {
			term.Warnf("Unable to save selected provider to defang server: %v", err)
		} else {
			term.Printf("%v is now the default provider for project %v and will auto-select next time if no other provider is specified. Use --provider=auto to reselect.", providerID, projectName)
		}
	}
	return "interactive prompt", nil
}
