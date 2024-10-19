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
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/aws/smithy-go"
	"github.com/bufbuild/connect-go"
	proj "github.com/compose-spec/compose-go/v2/types"
	"github.com/spf13/cobra"
)

const DEFANG_PORTAL_HOST = "portal.defang.dev"
const SERVICE_PORTAL_URL = "https://" + DEFANG_PORTAL_HOST + "/service"

const authNeeded = "auth-needed" // annotation to indicate that a command needs authorization
var authNeededAnnotation = map[string]string{authNeeded: ""}

// GLOBALS
var (
	client         cliClient.Client
	cluster        string
	colorMode      = ColorAuto
	doDebug        = false
	gitHubClientId = pkg.Getenv("DEFANG_CLIENT_ID", "7b41848ca116eac4b125") // GitHub OAuth app
	hasTty         = term.IsTerminal() && !pkg.GetenvBool("CI")
	nonInteractive = !hasTty
	provider       = cliClient.Provider(pkg.Getenv("DEFANG_PROVIDER", "auto"))
)

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
	if term.StdoutCanColor() { // TODO: should use DoColor(…) instead
		restore := term.EnableANSI()
		defer restore()
	}

	if err := RootCmd.ExecuteContext(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			term.Error("Error:", prettyError(err))
		}

		if err == cli.ErrDryRun {
			return nil
		}

		var derr *cli.ComposeError
		if errors.As(err, &derr) {
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
			if resp, err := client.GetServices(ctx); err == nil {
				projectName = resp.Project
			}
			printDefangHint("To deactivate a project, do:", "compose down --project-name "+projectName)
		}

		var cerr *cli.CancelError
		if errors.As(err, &cerr) {
			printDefangHint("Detached. The process will keep running.\nTo continue the logs from where you left off, do:", cerr.Error())
		}

		code := connect.CodeOf(err)
		if code == connect.CodeUnauthenticated {
			// All AWS errors are wrapped in OperationError
			var oe *smithy.OperationError
			if errors.As(err, &oe) {
				fmt.Println("Could not authenticate to the AWS service. Please check your AWS credentials and try again.")
			} else {
				printDefangHint("Please use the following command to log in:", "login")
			}
		}
		if code == connect.CodeFailedPrecondition && (strings.Contains(err.Error(), "EULA") || strings.Contains(err.Error(), "terms")) {
			printDefangHint("Please use the following command to see the Defang terms of service:", "terms")
		}
		return ExitCode(code)
	}

	if hasTty && term.HadWarnings() {
		fmt.Println("For help with warnings, check our FAQ at https://docs.defang.io/docs/faq")
	}

	if hasTty && !pkg.GetenvBool("DEFANG_HIDE_UPDATE") && rand.Intn(10) == 0 {
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

func SetupCommands(version string) {
	RootCmd.Version = version
	RootCmd.PersistentFlags().Var(&colorMode, "color", fmt.Sprintf(`colorize output; one of %v`, allColorModes))
	RootCmd.PersistentFlags().StringVarP(&cluster, "cluster", "s", cli.DefangFabric, "Defang cluster to connect to")
	RootCmd.PersistentFlags().MarkHidden("cluster")
	RootCmd.PersistentFlags().VarP(&provider, "provider", "P", fmt.Sprintf(`bring-your-own-cloud provider; one of %v`, cliClient.AllProviders()))
	RootCmd.PersistentFlags().BoolVarP(&cli.DoVerbose, "verbose", "v", false, "verbose logging") // backwards compat: only used by tail
	RootCmd.PersistentFlags().BoolVar(&doDebug, "debug", pkg.GetenvBool("DEFANG_DEBUG"), "debug logging for troubleshooting the CLI")
	RootCmd.PersistentFlags().BoolVar(&cli.DoDryRun, "dry-run", false, "dry run (don't actually change anything)")
	RootCmd.PersistentFlags().BoolVarP(&nonInteractive, "non-interactive", "T", !hasTty, "disable interactive prompts / no TTY")
	RootCmd.PersistentFlags().StringP("project-name", "p", "", "project name")
	RootCmd.PersistentFlags().StringP("cwd", "C", "", "change directory before running the command")
	_ = RootCmd.MarkPersistentFlagDirname("cwd")
	RootCmd.PersistentFlags().StringArrayP("file", "f", []string{}, `compose file path`)
	_ = RootCmd.MarkPersistentFlagFilename("file", "yml", "yaml")

	// Bootstrap command
	RootCmd.AddCommand(bootstrapCmd)
	bootstrapCmd.AddCommand(bootstrapDestroyCmd)
	bootstrapCmd.AddCommand(bootstrapDownCmd)
	bootstrapCmd.AddCommand(bootstrapRefreshCmd)
	bootstrapTearDownCmd.Flags().Bool("force", false, "force the teardown of the CD stack")
	bootstrapCmd.AddCommand(bootstrapTearDownCmd)
	bootstrapListCmd.Flags().Bool("remote", false, "invoke the command on the remote cluster")
	bootstrapCmd.AddCommand(bootstrapListCmd)
	bootstrapCmd.AddCommand(bootstrapCancelCmd)

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
	// loginCmd.Flags().Bool("skip-prompt", false, "skip the login prompt if already logged in"); TODO: Implement this
	RootCmd.AddCommand(loginCmd)

	// Whoami Command
	RootCmd.AddCommand(whoamiCmd)

	// Logout Command
	RootCmd.AddCommand(logoutCmd)

	// Generate Command
	RootCmd.AddCommand(generateCmd)
	RootCmd.AddCommand(newCmd)

	// Get Services Command
	lsCommand := makeComposeLsCmd()
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
	_ = configSetCmd.Flags().MarkHidden("name")

	configCmd.AddCommand(configSetCmd)

	configDeleteCmd.Flags().BoolP("name", "n", false, "name of the config(s) (backwards compat)")
	_ = configDeleteCmd.Flags().MarkHidden("name")
	configCmd.AddCommand(configDeleteCmd)

	configCmd.AddCommand(configListCmd)

	RootCmd.AddCommand(configCmd)
	RootCmd.AddCommand(restartCmd)

	RootCmd.AddCommand(setupComposeCommand())
	// Add up/down commands to the root as well
	RootCmd.AddCommand(makeComposeDownCmd())
	RootCmd.AddCommand(makeComposeUpCmd())

	// Debug Command
	debugCmd.Flags().String("etag", "", "deployment ID (ETag) of the service")
	RootCmd.AddCommand(debugCmd)

	// Tail Command
	tailCmd := makeComposeLogsCmd()
	tailCmd.Use = "tail"
	tailCmd.Aliases = []string{"logs"}
	RootCmd.AddCommand(tailCmd)

	// Delete Command
	deleteCmd.Flags().BoolP("name", "n", false, "name of the service(s) (backwards compat)")
	_ = deleteCmd.Flags().MarkHidden("name")
	deleteCmd.Flags().Bool("tail", false, "tail the service logs after deleting")
	RootCmd.AddCommand(deleteCmd)

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
		trackCmd(cmd, "Help", P{"args", args})
		origHelpFunc(cmd, args)
	})
}

var RootCmd = &cobra.Command{
	SilenceUsage:  true,
	SilenceErrors: true,
	Use:           "defang",
	Args:          cobra.NoArgs,
	Short:         "Defang CLI is used to develop, deploy, and debug your cloud services",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {

		term.SetDebug(doDebug)

		// Don't track/connect the completion commands
		if IsCompletionCommand(cmd) {
			return nil
		}

		// Use "defer" to track any errors that occur during the command
		defer func() {
			trackCmd(cmd, "Invoked", P{"args", args}, P{"err", err}, P{"non-interactive", nonInteractive}, P{"provider", provider})
		}()

		// Do this first, since any errors will be printed to the console
		switch colorMode {
		case ColorNever:
			term.ForceColor(false)
		case ColorAlways:
			term.ForceColor(true)
		}

		switch provider {
		case cliClient.ProviderAuto:
			if awsInEnv() {
				term.Warn("Using Defang playground, but AWS environment variables were detected; did you forget --provider=aws or DEFANG_PROVIDER=aws?")
			} else if doInEnv() {
				term.Warn("Using Defang playground, but DIGITALOCEAN_TOKEN environment variable was detected; did you forget --provider=digitalocean or DEFANG_PROVIDER=digitalocean?")
			}
			provider = cliClient.ProviderDefang
		case cliClient.ProviderAWS:
			if !awsInEnv() {
				term.Warn("AWS provider was selected, but AWS environment variables are not set")
			}
		case cliClient.ProviderDO:
			if !doInEnv() {
				term.Warn("DigitalOcean provider was selected, but DIGITALOCEAN_TOKEN environment variable is not set")
			}
		case cliClient.ProviderDefang:
			// Ignore any env vars when explicitly using the Defang playground provider
		}

		cwd, _ := cmd.Flags().GetString("cwd")
		if cwd != "" {
			// Change directory before running the command
			if err = os.Chdir(cwd); err != nil {
				return err
			}
		}
		loader := configureLoader(cmd)
		client = cli.NewClient(cmd.Context(), cluster, provider, loader)

		if v, err := client.GetVersions(cmd.Context()); err == nil {
			version := cmd.Root().Version // HACK to avoid circular dependency with RootCmd
			term.Debug("Fabric:", v.Fabric, "CLI:", version, "CLI-Min:", v.CliMin)
			if hasTty && isNewer(version, v.CliMin) {
				term.Warn("Your CLI version is outdated. Please upgrade to the latest version by running:\n\ndefang upgrade")
				os.Setenv("DEFANG_HIDE_UPDATE", "1") // hide the upgrade hint at the end
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

				defer func() { trackCmd(nil, "Login", P{"reason", err}) }()
				if err = cli.InteractiveLogin(cmd.Context(), client, gitHubClientId, cluster); err != nil {
					return err
				}

				// FIXME: the new login might have changed the tenant, so we should reload the project
				client = cli.NewClient(cmd.Context(), cluster, provider, loader) // reconnect with the new token
				if err = client.CheckLoginAndToS(cmd.Context()); err == nil {    // recheck (new token = new user)
					return nil // success
				}
			}

			// Check if the user has agreed to the terms of service and show a prompt if needed
			if connect.CodeOf(err) == connect.CodeFailedPrecondition {
				term.Warn(prettyError(err))

				defer func() { trackCmd(nil, "Terms", P{"reason", err}) }()
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
		if nonInteractive {
			if err := cli.NonInteractiveLogin(cmd.Context(), client, cluster); err != nil {
				return err
			}
		} else {
			err := cli.InteractiveLogin(cmd.Context(), client, gitHubClientId, cluster)
			if err != nil {
				return err
			}

			printDefangHint("To generate a sample service, do:", "generate")
		}
		return nil
	},
}

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Args:  cobra.NoArgs,
	Short: "Show the current user",
	RunE: func(cmd *cobra.Command, args []string) error {
		str, err := cli.Whoami(cmd.Context(), client)
		if err != nil {
			return err
		}

		term.Infof(str)
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
		err := cli.GenerateLetsEncryptCert(cmd.Context(), client)
		if err != nil {
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
				}, &sample); err != nil {
					return err
				}
				if sample == generateWithAI {
					if err := survey.AskOne(&survey.Select{
						Message: "Choose the language you'd like to use:",
						Options: cli.SupportedLanguages,
						Help:    "The project code will be in the language you choose here.",
					}, &language); err != nil {
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
		err := survey.Ask(qs, &prompt)
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

		Track("Generate Started", P{"language", language}, P{"sample", sample}, P{"description", prompt.Description}, P{"folder", prompt.Folder})

		// Check if the current folder is empty
		if empty, err := pkg.IsDirEmpty(prompt.Folder); !os.IsNotExist(err) && !empty {
			term.Warnf("The folder %q is not empty. We recommend running this command in an empty folder.", prompt.Folder)
		}

		if sample != "" {
			term.Info("Fetching sample from the Defang repository...")
			err := cli.InitFromSamples(cmd.Context(), prompt.Folder, []string{sample})
			if err != nil {
				return err
			}
		} else {
			term.Info("Working on it. This may take 1 or 2 minutes...")
			_, err := cli.GenerateWithAI(cmd.Context(), client, language, prompt.Folder, prompt.Description)
			if err != nil {
				return err
			}
		}

		term.Info("Code generated successfully in folder", prompt.Folder)

		cmdd := exec.Command("code", prompt.Folder)
		err = cmdd.Start()
		if err != nil {
			term.Debug("unable to launch VS Code:", err)
			// TODO: should we use EDITOR env var instead?
		}

		cd := ""
		if prompt.Folder != "." {
			cd = "`cd " + prompt.Folder + "` and "
		}

		// Load the project and check for empty environment variables
		loaderOptions := compose.LoaderOptions{
			ConfigPaths: []string{filepath.Join(prompt.Folder, "compose.yaml")},
		}
		loader := compose.NewLoaderWithOptions(loaderOptions)
		project, _ := loader.LoadProject(cmd.Context())

		var envInstructions []string
		for _, envVar := range collectUnsetEnvVars(project) {
			envInstructions = append(envInstructions, "config create "+envVar)
		}

		if len(envInstructions) > 0 {
			printDefangHint("Check the files in your favorite editor.\nTo deploy the service, do "+cd, envInstructions...)
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

func collectUnsetEnvVars(project *proj.Project) []string {
	var envVars []string
	if project != nil {
		for _, service := range project.Services {
			for key, value := range service.Environment {
				if value == nil {
					envVars = append(envVars, key)
				}
			}
		}
	}
	return envVars
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

		// Make sure we have a project to set config for before asking for a value
		_, err := client.LoadProjectName(cmd.Context())
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
		} else {
			// Prompt for sensitive value
			var sensitivePrompt = &survey.Password{
				Message: fmt.Sprintf("Enter value for %q:", name),
				Help:    "The value will be stored securely and cannot be retrieved later.",
			}

			err := survey.AskOne(sensitivePrompt, &value)
			if err != nil {
				return err
			}
		}

		if err := cli.ConfigSet(cmd.Context(), client, name, value); err != nil {
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
		if err := cli.ConfigDelete(cmd.Context(), client, names...); err != nil {
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
		return cli.ConfigList(cmd.Context(), client)
	},
}

var debugCmd = &cobra.Command{
	Use:         "debug [SERVICE...]",
	Annotations: authNeededAnnotation,
	Hidden:      true,
	Short:       "Debug a build, deployment, or service failure",
	RunE: func(cmd *cobra.Command, args []string) error {
		etag, _ := cmd.Flags().GetString("etag")

		project, err := client.LoadProject(cmd.Context())
		if err != nil {
			return err
		}

		return cli.Debug(cmd.Context(), client, etag, project, args)
	},
}

var deleteCmd = &cobra.Command{
	Use:         "delete SERVICE...",
	Annotations: authNeededAnnotation,
	Args:        cobra.MinimumNArgs(1),
	Aliases:     []string{"del", "rm", "remove"},
	Short:       "Delete a service from the cluster",
	Deprecated:  "use 'compose down' instead",
	RunE: func(cmd *cobra.Command, names []string) error {
		var tail, _ = cmd.Flags().GetBool("tail")

		since := time.Now()
		etag, err := cli.Delete(cmd.Context(), client, names...)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				// Show a warning (not an error) if the service was not found
				term.Warn(prettyError(err))
				return nil
			}
			return err
		}

		term.Info("Deleted service", names, "with deployment ID", etag)

		if !tail {
			printDefangHint("To track the update, do:", "tail --etag "+etag)
			return nil
		}

		term.Info("Tailing logs for update; press Ctrl+C to detach:")
		tailParams := cli.TailOptions{
			Etag:  etag,
			Since: since,
			Raw:   false,
		}
		return cli.Tail(cmd.Context(), client, tailParams)
	},
}

var restartCmd = &cobra.Command{
	Use:         "restart SERVICE...",
	Annotations: authNeededAnnotation,
	Args:        cobra.MinimumNArgs(1),
	Short:       "Restart one or more services",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errors.New("Command 'restart' is deprecated, use 'up' instead")
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
		return cli.Token(cmd.Context(), client, gitHubClientId, types.DEFAULT_TENANT, expires, scope.Scope(s))
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

var bootstrapCmd = &cobra.Command{
	Use:     "cd",
	Aliases: []string{"bootstrap"},
	Short:   "Manually run a command with the CD task (for BYOC only)",
	Hidden:  true,
}

var bootstrapDestroyCmd = &cobra.Command{
	Use:   "destroy",
	Args:  cobra.NoArgs, // TODO: set MaximumNArgs(1),
	Short: "Destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.BootstrapCommand(cmd.Context(), client, "destroy")
	},
}

var bootstrapDownCmd = &cobra.Command{
	Use:   "down",
	Args:  cobra.NoArgs, // TODO: set MaximumNArgs(1),
	Short: "Refresh and then destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.BootstrapCommand(cmd.Context(), client, "down")
	},
}

var bootstrapRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Args:  cobra.NoArgs, // TODO: set MaximumNArgs(1),
	Short: "Refresh the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.BootstrapCommand(cmd.Context(), client, "refresh")
	},
}

var bootstrapCancelCmd = &cobra.Command{
	Use:   "cancel",
	Args:  cobra.NoArgs, // TODO: set MaximumNArgs(1),
	Short: "Cancel the current CD operation",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.BootstrapCommand(cmd.Context(), client, "cancel")
	},
}

var bootstrapTearDownCmd = &cobra.Command{
	Use:   "teardown",
	Args:  cobra.NoArgs,
	Short: "Destroy the CD cluster without destroying the services",
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		return cli.TearDown(cmd.Context(), client, force)
	},
}

var bootstrapListCmd = &cobra.Command{
	Use:     "ls",
	Args:    cobra.NoArgs,
	Aliases: []string{"list"},
	Short:   "List all the projects and stacks in the CD cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		remote, _ := cmd.Flags().GetBool("remote")

		if remote {
			return cli.BootstrapCommand(cmd.Context(), client, "list")
		}
		return cli.BootstrapLocalList(cmd.Context(), client)
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
		return cli.Upgrade(cmd.Context())
	},
}

func configureLoader(cmd *cobra.Command) compose.Loader {
	f := cmd.Flags()
	o := compose.LoaderOptions{}
	var err error

	o.ConfigPaths, err = f.GetStringArray("file")
	if err != nil {
		panic(err)
	}

	o.ProjectName, err = f.GetString("project-name")
	if err != nil {
		panic(err)
	}
	return compose.NewLoaderWithOptions(o)
}

func awsInEnv() bool {
	return os.Getenv("AWS_PROFILE") != "" || os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_SECRET_ACCESS_KEY") != ""
}

func doInEnv() bool {
	return os.Getenv("DIGITALOCEAN_ACCESS_TOKEN") != "" || os.Getenv("DIGITALOCEAN_TOKEN") != ""
}
