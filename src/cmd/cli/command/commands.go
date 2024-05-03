package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/aws/smithy-go"
	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli"
	cliClient "github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/scope"
	"github.com/defang-io/defang/src/pkg/term"
	"github.com/defang-io/defang/src/pkg/types"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
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
	gitHubClientId = pkg.Getenv("DEFANG_CLIENT_ID", "7b41848ca116eac4b125") // GitHub OAuth app
	hasTty         = term.IsTerminal && !pkg.GetenvBool("CI")
	nonInteractive = !hasTty
	provider       = cliClient.Provider(pkg.Getenv("DEFANG_PROVIDER", "auto"))
)

func prettyError(err error) error {
	// To avoid printing the internal gRPC error code
	var cerr *connect.Error
	if errors.As(err, &cerr) {
		term.Debug(" - Server error:", err)
		err = errors.Unwrap(err)
	}
	return err

}

func Execute(ctx context.Context) error {
	if err := RootCmd.ExecuteContext(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			term.Error("Error:", prettyError(err))
		}

		var derr *cli.ComposeError
		if errors.As(err, &derr) {
			compose := "compose"
			fileFlag := composeCmd.Flag("file")
			if fileFlag.Changed {
				compose += " -f " + fileFlag.Value.String()
			}
			printDefangHint("Fix the error and try again. To validate the compose file, use:", compose+" config")
		}

		if strings.Contains(err.Error(), "secret") {
			printDefangHint("To manage sensitive service config, use:", "config")
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
				fmt.Println("Could not authenticate to the AWS service. Please check your aws credentials and try again.")
			} else {
				printDefangHint("Please use the following command to log in:", "login")
			}
		}
		if code == connect.CodeFailedPrecondition && (strings.Contains(err.Error(), "EULA") || strings.Contains(err.Error(), "terms")) {
			printDefangHint("Please use the following command to see the Defang terms of service:", "terms")
		}

		return ExitCode(code)
	}

	if hasTty && term.HadWarnings {
		fmt.Println("For help with warnings, check our FAQ at https://docs.defang.io/docs/faq")
	}

	if hasTty && !pkg.GetenvBool("DEFANG_HIDE_UPDATE") && rand.Intn(10) == 0 {
		if latest, err := GetLatestVersion(ctx); err == nil && semver.Compare(GetCurrentVersion(), latest) < 0 {
			term.Debug(" - Latest Version:", latest, "Current Version:", GetCurrentVersion())
			fmt.Println("A newer version of the CLI is available at https://github.com/defang-io/defang/releases/latest")
			if rand.Intn(10) == 0 && !pkg.GetenvBool("DEFANG_HIDE_HINTS") {
				fmt.Println("To silence these notices, do: export DEFANG_HIDE_UPDATE=1")
			}
		}
	}

	return nil
}

func SetupCommands(version string) {
	defangFabric := pkg.Getenv("DEFANG_FABRIC", cli.DefaultCluster)

	RootCmd.Version = version
	RootCmd.PersistentFlags().Var(&colorMode, "color", `colorize output; "auto", "always" or "never"`)
	RootCmd.PersistentFlags().StringVarP(&cluster, "cluster", "s", defangFabric, "Defang cluster to connect to")
	RootCmd.PersistentFlags().VarP(&provider, "provider", "P", `cloud provider to use; use "aws" for bring-your-own-cloud`)
	RootCmd.PersistentFlags().BoolVarP(&cli.DoVerbose, "verbose", "v", false, "verbose logging") // backwards compat: only used by tail
	RootCmd.PersistentFlags().BoolVar(&term.DoDebug, "debug", false, "debug logging for troubleshooting the CLI")
	RootCmd.PersistentFlags().BoolVar(&cli.DoDryRun, "dry-run", false, "dry run (don't actually change anything)")
	RootCmd.PersistentFlags().BoolVarP(&nonInteractive, "non-interactive", "T", !hasTty, "disable interactive prompts / no TTY")
	RootCmd.PersistentFlags().StringP("cwd", "C", "", "change directory before running the command")
	RootCmd.MarkPersistentFlagDirname("cwd")
	RootCmd.PersistentFlags().StringP("file", "f", "", `compose file path`)
	RootCmd.MarkPersistentFlagFilename("file", "yml", "yaml")

	// Bootstrap command
	RootCmd.AddCommand(bootstrapCmd)
	bootstrapCmd.AddCommand(bootstrapDestroyCmd)
	bootstrapCmd.AddCommand(bootstrapDownCmd)
	bootstrapCmd.AddCommand(bootstrapRefreshCmd)
	bootstrapCmd.AddCommand(bootstrapTearDownCmd)
	bootstrapCmd.AddCommand(bootstrapListCmd)
	bootstrapCmd.AddCommand(bootstrapCancelCmd)

	// Eula command
	tosCmd.Flags().Bool("agree-tos", false, "Agree to the Defang terms of service")
	RootCmd.AddCommand(tosCmd)

	// Token command
	tokenCmd.Flags().Duration("expires", 24*time.Hour, "Validity duration of the token")
	tokenCmd.Flags().String("scope", "", fmt.Sprintf("Scope of the token; one of %v (required)", scope.All()))
	tokenCmd.MarkFlagRequired("scope")
	RootCmd.AddCommand(tokenCmd)

	// Login Command
	// loginCmd.Flags().Bool("skip-prompt", false, "Skip the login prompt if already logged in"); TODO: Implement this
	RootCmd.AddCommand(loginCmd)

	// Whoami Command
	RootCmd.AddCommand(whoamiCmd)

	// Logout Command
	RootCmd.AddCommand(logoutCmd)

	// Generate Command
	//generateCmd.Flags().StringP("name", "n", "service1", "Name of the service")
	RootCmd.AddCommand(generateCmd)

	// Get Services Command
	getServicesCmd.Flags().BoolP("long", "l", false, "Show more details")
	RootCmd.AddCommand(getServicesCmd)

	// Get Status Command
	RootCmd.AddCommand(getVersionCmd)

	// Config Command (was: secrets)
	configSetCmd.Flags().BoolP("name", "n", false, "Name of the config (backwards compat)")
	configSetCmd.Flags().MarkHidden("name")
	configCmd.AddCommand(configSetCmd)

	configDeleteCmd.Flags().BoolP("name", "n", false, "Name of the config(s) (backwards compat)")
	configDeleteCmd.Flags().MarkHidden("name")
	configCmd.AddCommand(configDeleteCmd)

	configCmd.AddCommand(configListCmd)

	RootCmd.AddCommand(configCmd)
	RootCmd.AddCommand(restartCmd)

	// Compose Command
	// composeCmd.Flags().Bool("compatibility", false, "Run compose in backward compatibility mode"); TODO: Implement compose option
	// composeCmd.Flags().String("env-file", "", "Specify an alternate environment file."); TODO: Implement compose option
	// composeCmd.Flags().Int("parallel", -1, "Control max parallelism, -1 for unlimited (default -1)"); TODO: Implement compose option
	// composeCmd.Flags().String("profile", "", "Specify a profile to enable"); TODO: Implement compose option
	// composeCmd.Flags().String("project-directory", "", "Specify an alternate working directory"); TODO: Implement compose option
	// composeCmd.Flags().StringP("project", "p", "", "Compose project name"); TODO: Implement compose option
	composeUpCmd.Flags().Bool("tail", false, "Tail the service logs after updating") // obsolete, but keep for backwards compatibility
	composeUpCmd.Flags().MarkHidden("tail")
	composeUpCmd.Flags().Bool("force", false, "Force a build of the image even if nothing has changed")
	composeUpCmd.Flags().BoolP("detach", "d", false, "Run in detached mode")
	composeCmd.AddCommand(composeUpCmd)
	composeCmd.AddCommand(composeConfigCmd)
	composeDownCmd.Flags().Bool("tail", false, "Tail the service logs after deleting") // obsolete, but keep for backwards compatibility
	composeDownCmd.Flags().BoolP("detach", "d", false, "Run in detached mode")
	composeDownCmd.Flags().MarkHidden("tail")
	composeCmd.AddCommand(composeDownCmd)
	composeStartCmd.Flags().Bool("force", false, "Force a build of the image even if nothing has changed")
	composeCmd.AddCommand(composeStartCmd)
	RootCmd.AddCommand(composeCmd)
	composeCmd.AddCommand(composeRestartCmd)
	composeCmd.AddCommand(composeStopCmd)

	// Tail Command
	tailCmd.Flags().StringP("name", "n", "", "Name of the service")
	tailCmd.Flags().String("etag", "", "ETag or deployment ID of the service")
	tailCmd.Flags().BoolP("raw", "r", false, "Show raw (unparsed) logs")
	tailCmd.Flags().String("since", "5s", "Show logs since duration/time")
	RootCmd.AddCommand(tailCmd)

	// Delete Command
	deleteCmd.Flags().BoolP("name", "n", false, "Name of the service(s) (backwards compat)")
	deleteCmd.Flags().MarkHidden("name")
	deleteCmd.Flags().Bool("tail", false, "Tail the service logs after deleting")
	RootCmd.AddCommand(deleteCmd)

	// Send Command
	sendCmd.Flags().StringP("subject", "n", "", "Subject to send the message to (required)")
	sendCmd.Flags().StringP("type", "t", "", "Type of message to send (required)")
	sendCmd.Flags().String("id", "", "ID of the message")
	sendCmd.Flags().StringP("data", "d", "", "String data to send")
	sendCmd.Flags().StringP("content-type", "c", "", "Content-Type of the data")
	sendCmd.MarkFlagRequired("subject")
	sendCmd.MarkFlagRequired("type")
	RootCmd.AddCommand(sendCmd)

	// Cert management
	// TODO: Add list, renew etc.
	certCmd.AddCommand(certGenerateCmd)
	RootCmd.AddCommand(certCmd)

	if term.CanColor {
		restore := term.EnableANSI()
		cobra.OnFinalize(restore)

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
	Short:         "Defang CLI manages services on the Defang cluster",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
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
				provider = cliClient.ProviderAWS
			} else {
				provider = cliClient.ProviderDefang
			}
		case cliClient.ProviderAWS:
			if !awsInEnv() {
				term.Warn(" ! AWS provider was selected, but AWS environment variables are not set")
			}
		case cliClient.ProviderDefang:
			if awsInEnv() {
				term.Warn(" ! Using Defang provider, but AWS environment variables were detected; use --provider")
			}
		}

		cwd, _ := cmd.Flags().GetString("cwd")
		if cwd != "" {
			// Change directory before running the command
			if err = os.Chdir(cwd); err != nil {
				return err
			}
		}

		composeFilePath, _ := cmd.Flags().GetString("file")
		loader := cli.ComposeLoader{ComposeFilePath: composeFilePath}
		client = cli.NewClient(cluster, provider, loader)

		if v, err := client.GetVersions(cmd.Context()); err == nil {
			version := "v" + cmd.Root().Version // HACK to avoid circular dependency with RootCmd
			term.Debug(" - Fabric:", v.Fabric, "CLI:", version, "Min CLI:", v.CliMin)
			if hasTty && semver.Compare(version, v.CliMin) < 0 {
				term.Warn(" ! Your CLI version is outdated. Please update to the latest version.")
				os.Setenv("DEFANG_HIDE_UPDATE", "1") // hide the update hint at the end
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
				term.Warn(" !", prettyError(err))

				if err = cli.InteractiveLogin(cmd.Context(), client, gitHubClientId, cluster); err != nil {
					return err
				}

				// FIXME: the new login might have changed the tenant, so we should reload the project
				client = cli.NewClient(cluster, provider, loader)             // reconnect with the new token
				if err = client.CheckLoginAndToS(cmd.Context()); err == nil { // recheck (new token = new user)
					return nil // success
				}
			}

			// Check if the user has agreed to the terms of service and show a prompt if needed
			if connect.CodeOf(err) == connect.CodeFailedPrecondition {
				term.Warn(" !", prettyError(err))
				if err = cli.InteractiveAgreeToS(cmd.Context(), client); err != nil {
					return err
				}
			}
		}
		return err
	},
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Args:  cobra.NoArgs,
	Short: "Authenticate to the Defang cluster",
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
		err := cli.Whoami(cmd.Context(), client) // always prints
		if err != nil {
			return err
		}
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
	Short:   "Generate an letsencrypt certificate",
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
	Args:    cobra.NoArgs,
	Aliases: []string{"gen", "new", "init"},
	Short:   "Generate a sample Defang project in the current folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		if nonInteractive {
			return errors.New("cannot run in non-interactive mode")
		}

		var qs = []*survey.Question{
			{
				Name: "language",
				Prompt: &survey.Select{
					Message: "Choose the language you'd like to use:",
					Options: []string{"Nodejs", "Golang", "Python"},
					Default: "Nodejs",
					Help:    "The generated code will be in the language you choose here.",
				},
			},
			{
				Name: "description",
				Prompt: &survey.Input{
					Message: "Please describe the service you'd like to build:",
					Help: `Here are some example prompts you can use:
	"A simple 'hello world' function"
	"A service with 2 endpoints, one to upload and the other to download a file from AWS S3"
	"A service with a default endpoint that returns an HTML page with a form asking for the user's name and then a POST endpoint to handle the form post when the user clicks the 'submit' button\"
Generate will write files in the current folder. You can edit them and then deploy using 'defang compose up --tail' when ready.`,
				},
				Validate: survey.MinLength(5),
			},
			{
				Name: "folder",
				Prompt: &survey.Input{
					Message: "What folder would you like to create the service in?",
					Default: "service1",
					Help:    "The generated code will be in the folder you choose here. If the folder does not exist, it will be created.",
				},
				Validate: survey.Required,
			},
		}

		prompt := struct {
			Language    string // or you can tag fields to match a specific name
			Description string
			Folder      string
		}{}

		// ask the questions
		err := survey.Ask(qs, &prompt)
		if err != nil {
			return err
		}

		if client.CheckLoginAndToS(cmd.Context()) != nil {
			// The user is either not logged in or has not agreed to the terms of service; ask for agreement to the terms now
			if err := cli.InteractiveAgreeToS(cmd.Context(), client); err != nil {
				// This might fail because the user did not log in. This is fine: we won't persist the terms agreement, but can proceed with the generation
				if connect.CodeOf(err) != connect.CodeUnauthenticated {
					return err
				}
			}
		}

		Track("Generate Started", P{"language", prompt.Language}, P{"description", prompt.Description}, P{"folder", prompt.Folder})

		// create the folder if needed
		cd := ""
		if prompt.Folder != "." {
			cd = "`cd " + prompt.Folder + "` and "
			os.MkdirAll(prompt.Folder, 0755)
			if err := os.Chdir(prompt.Folder); err != nil {
				return err
			}
		}

		// Check if the current folder is empty
		if empty, err := pkg.IsDirEmpty("."); !empty || err != nil {
			term.Warn(" ! The folder is not empty. Files may be overwritten. Press Ctrl+C to abort.")
		}

		term.Info(" * Working on it. This may take 1 or 2 minutes...")
		_, err = cli.Generate(cmd.Context(), client, prompt.Language, prompt.Description)
		if err != nil {
			return err
		}

		term.Info(" * Code generated successfully in folder", prompt.Folder)

		// TODO: should we use EDITOR env var instead?
		cmdd := exec.Command("code", ".")
		err = cmdd.Start()
		if err != nil {
			term.Debug(" - unable to launch VS Code:", err)
		}

		printDefangHint("Check the files in your favorite editor.\nTo deploy the service, "+cd+"do:", "compose up")
		return nil
	},
}

var getServicesCmd = &cobra.Command{
	Use:         "services",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Aliases:     []string{"getServices", "ls", "list"},
	Short:       "Get list of services on the cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		long, _ := cmd.Flags().GetBool("long")

		err := cli.GetServices(cmd.Context(), client, long)
		if err != nil {
			return err
		}

		if !long {
			printDefangHint("To see more information about your services, do:", cmd.CalledAs()+" -l")
		}

		return nil
	},
}

var getVersionCmd = &cobra.Command{
	Use:     "version",
	Args:    cobra.NoArgs,
	Aliases: []string{"ver", "stat", "status"}, // for backwards compatibility
	Short:   "Get version information for the CLI and Fabric service",
	RunE: func(cmd *cobra.Command, args []string) error {
		term.Print(term.BrightCyan, "Defang CLI:    ")
		fmt.Println(GetCurrentVersion())

		term.Print(term.BrightCyan, "Latest CLI:    ")
		ver, err := GetLatestVersion(cmd.Context())
		fmt.Println(ver)

		term.Print(term.BrightCyan, "Defang Fabric: ")
		ver, err2 := cli.GetVersion(cmd.Context(), client)
		fmt.Println(ver)
		return errors.Join(err, err2)
	},
}

var tailCmd = &cobra.Command{
	Use:         "tail",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Short:       "Tail logs from one or more services",
	RunE: func(cmd *cobra.Command, args []string) error {
		var name, _ = cmd.Flags().GetString("name")
		var etag, _ = cmd.Flags().GetString("etag")
		var raw, _ = cmd.Flags().GetBool("raw")
		var since, _ = cmd.Flags().GetString("since")

		ts, err := cli.ParseTimeOrDuration(since)
		if err != nil {
			return fmt.Errorf("invalid duration or time: %w", err)
		}

		ts = ts.UTC()
		term.Info(" * Showing logs since", ts.Format(time.RFC3339Nano), "; press Ctrl+C to stop:")
		return cli.Tail(cmd.Context(), client, name, etag, ts, raw)
	},
}

var configCmd = &cobra.Command{
	Use:     "config", // like Docker
	Args:    cobra.NoArgs,
	Aliases: []string{"secrets", "secret", "env"},
	Short:   "Add, update, or delete service config",
}

var configSetCmd = &cobra.Command{
	Use:         "create CONFIG", // like Docker
	Annotations: authNeededAnnotation,
	Args:        cobra.ExactArgs(1),
	Aliases:     []string{"set", "add", "put"},
	Short:       "Adds or updates a sensitive config value",
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		var value string
		if !nonInteractive {
			// Prompt for sensitive value
			var sensitivePrompt = &survey.Password{
				Message: fmt.Sprintf("Enter value for %q:", name),
				Help:    "The value will be stored securely and cannot be retrieved later.",
			}

			err := survey.AskOne(sensitivePrompt, &value)
			if err != nil {
				return err
			}
		} else {
			bytes, err := io.ReadAll(os.Stdin)
			if err != nil && err != io.EOF {
				return fmt.Errorf("failed reading the value from non-terminal: %w", err)
			}
			value = strings.TrimSuffix(string(bytes), "\n")
		}

		if err := cli.ConfigSet(cmd.Context(), client, name, value); err != nil {
			return err
		}
		term.Info(" * Updated value for", name)

		printDefangHint("To update the deployed values, do:", "compose start")
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
				term.Warn(" !", prettyError(err))
				return nil
			}
			return err
		}
		term.Info(" * Deleted", names)

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

var composeCmd = &cobra.Command{
	Use:     "compose",
	Aliases: []string{"stack"},
	Args:    cobra.NoArgs,
	Short:   "Work with local Compose files",
}

func printPlaygroundPortalServiceURLs(serviceInfos []*defangv1.ServiceInfo) {
	// We can only show services deployed to the prod1 defang SaaS environment.
	if provider == cliClient.ProviderDefang && cluster == cli.DefaultCluster {
		term.Info(" * Monitor your services' status in the defang portal")
		for _, serviceInfo := range serviceInfos {
			fmt.Println("   -", SERVICE_PORTAL_URL+"/"+serviceInfo.Service.Name)
		}
	}
}

func printEndpoints(serviceInfos []*defangv1.ServiceInfo) {
	for _, serviceInfo := range serviceInfos {
		andEndpoints := ""
		if len(serviceInfo.Endpoints) > 0 {
			andEndpoints = "and will be available at:"
		}
		term.Info(" * Service", serviceInfo.Service.Name, "is in state", serviceInfo.Status, andEndpoints)
		for i, endpoint := range serviceInfo.Endpoints {
			if serviceInfo.Service.Ports[i].Mode == defangv1.Mode_INGRESS {
				endpoint = "https://" + endpoint
			}
			fmt.Println("   -", endpoint)
		}
		if serviceInfo.Service.Domainname != "" {
			if serviceInfo.ZoneId != "" {
				fmt.Println("   -", "https://"+serviceInfo.Service.Domainname)
			} else {
				fmt.Println("   -", "https://"+serviceInfo.Service.Domainname+" (after ACME cert activation)")
			}
		}
	}
}

var composeUpCmd = &cobra.Command{
	Use:         "up",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Like 'start' but immediately tracks the progress of the deployment",
	RunE: func(cmd *cobra.Command, args []string) error {
		var force, _ = cmd.Flags().GetBool("force")
		var detach, _ = cmd.Flags().GetBool("detach")

		since := time.Now()
		deploy, err := cli.ComposeStart(cmd.Context(), client, force)
		if err != nil {
			return err
		}

		printPlaygroundPortalServiceURLs(deploy.Services)
		printEndpoints(deploy.Services) // TODO: do this at the end

		if detach {
			term.Info(" * Done.")
			return nil
		}

		etag := deploy.Etag
		services := "all services"
		if etag != "" {
			services = "deployment ID " + etag
		}

		term.Info(" * Tailing logs for", services, "; press Ctrl+C to detach:")
		err = cli.Tail(cmd.Context(), client, "", etag, since, false)
		if err != nil {
			return err
		}
		term.Info(" * Done.")
		return nil
	},
}

var composeStartCmd = &cobra.Command{
	Use:         "start",
	Aliases:     []string{"deploy"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Reads a Compose file and deploys services to the cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		var force, _ = cmd.Flags().GetBool("force")

		deploy, err := cli.ComposeStart(cmd.Context(), client, force)
		if err != nil {
			return err
		}

		printPlaygroundPortalServiceURLs(deploy.Services)
		printEndpoints(deploy.Services) // TODO: do this at the end

		command := "tail"
		if deploy.Etag != "" {
			command += " --etag " + deploy.Etag
		}
		printDefangHint("To track the update, do:", command)
		return nil
	},
}

var composeRestartCmd = &cobra.Command{
	Use:         "restart",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Reads a Compose file and restarts its services",
	RunE: func(cmd *cobra.Command, args []string) error {
		etag, err := cli.ComposeRestart(cmd.Context(), client)
		if err != nil {
			return err
		}
		term.Info(" * Restarted services with deployment ID", etag)
		return nil
	},
}

var composeStopCmd = &cobra.Command{
	Use:         "stop",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Reads a Compose file and stops its services",
	RunE: func(cmd *cobra.Command, args []string) error {
		etag, err := cli.ComposeStop(cmd.Context(), client)
		if err != nil {
			return err
		}
		term.Info(" * Stopped services with deployment ID", etag)
		return nil
	},
}

var composeDownCmd = &cobra.Command{
	Use:         "down",
	Aliases:     []string{"rm"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Like 'stop' but also deprovisions the services from the cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		var detach, _ = cmd.Flags().GetBool("detach")

		since := time.Now()
		etag, err := cli.ComposeDown(cmd.Context(), client)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				// Show a warning (not an error) if the service was not found
				term.Warn(" !", prettyError(err))
				return nil
			}
			return err
		}

		term.Info(" * Deleted services, deployment ID", etag)

		if detach {
			printDefangHint("To track the update, do:", "tail --etag "+etag)
			return nil
		}

		err = cli.Tail(cmd.Context(), client, "", etag, since, false)
		if err != nil {
			return err
		}
		term.Info(" * Done.")
		return nil

	},
}

var composeConfigCmd = &cobra.Command{
	Use:   "config",
	Args:  cobra.NoArgs, // TODO: takes optional list of service names
	Short: "Reads a Compose file and shows the generated config",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli.DoDryRun = true // config is like start in a dry run
		// force=false to calculate the digest
		if _, err := cli.ComposeStart(cmd.Context(), client, false); !errors.Is(err, cli.ErrDryRun) {
			return err
		}
		return nil
	},
}

var deleteCmd = &cobra.Command{
	Use:         "delete SERVICE...",
	Annotations: authNeededAnnotation,
	Args:        cobra.MinimumNArgs(1),
	Aliases:     []string{"del", "rm", "remove"},
	Short:       "Delete a service from the cluster",
	RunE: func(cmd *cobra.Command, names []string) error {
		var tail, _ = cmd.Flags().GetBool("tail")

		since := time.Now()
		etag, err := cli.Delete(cmd.Context(), client, names...)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				// Show a warning (not an error) if the service was not found
				term.Warn(" !", prettyError(err))
				return nil
			}
			return err
		}

		term.Info(" * Deleted service", names, "with deployment ID", etag)

		if !tail {
			printDefangHint("To track the update, do:", "tail --etag "+etag)
			return nil
		}

		term.Info(" * Tailing logs for update; press Ctrl+C to detach:")
		return cli.Tail(cmd.Context(), client, "", etag, since, false)
	},
}

var restartCmd = &cobra.Command{
	Use:         "restart SERVICE...",
	Annotations: authNeededAnnotation,
	Args:        cobra.MinimumNArgs(1),
	Short:       "Restart one or more services",
	RunE: func(cmd *cobra.Command, args []string) error {
		etag, err := cli.Restart(cmd.Context(), client, args...)
		if err != nil {
			return err
		}
		term.Info(" * Restarted service", args, "with deployment ID", etag)
		return nil
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
		term.Info(" * Successfully logged out")
		return nil
	},
}

var bootstrapCmd = &cobra.Command{
	Use:     "cd",
	Aliases: []string{"bootstrap"},
	Args:    cobra.NoArgs,
	Short:   "Manually run a command with the CD task",
}

var bootstrapDestroyCmd = &cobra.Command{
	Use:   "destroy",
	Args:  cobra.NoArgs,
	Short: "Destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.BootstrapCommand(cmd.Context(), client, "destroy")
	},
}

var bootstrapDownCmd = &cobra.Command{
	Use:   "down",
	Args:  cobra.NoArgs,
	Short: "Refresh and then destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.BootstrapCommand(cmd.Context(), client, "down")
	},
}

var bootstrapRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Args:  cobra.NoArgs,
	Short: "Refresh the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.BootstrapCommand(cmd.Context(), client, "refresh")
	},
}

var bootstrapTearDownCmd = &cobra.Command{
	Use:   "teardown",
	Args:  cobra.NoArgs,
	Short: "Destroy the CD cluster without destroying the services",
	RunE: func(cmd *cobra.Command, args []string) error {
		term.Warn(` ! Deleting the CD cluster; this does not delete the services!`)
		return cli.TearDown(cmd.Context(), client)
	},
}

var bootstrapListCmd = &cobra.Command{
	Use:     "ls",
	Args:    cobra.NoArgs,
	Aliases: []string{"list"},
	Short:   "List all the projects and stacks in the CD cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.BootstrapList(cmd.Context(), client)
	},
}

var bootstrapCancelCmd = &cobra.Command{
	Use:   "cancel",
	Args:  cobra.NoArgs,
	Short: "Cancel the current CD operation",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.BootstrapCommand(cmd.Context(), client, "cancel")
	},
}

var tosCmd = &cobra.Command{
	Use:         "terms",
	Aliases:     []string{"tos", "eula", "tac", "tou"},
	Annotations: authNeededAnnotation, // TODO: only need auth when agreeing to the terms
	Args:        cobra.NoArgs,
	Short:       "Read and/or agree the Defang terms of service",
	RunE: func(cmd *cobra.Command, args []string) error {
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

func awsInEnv() bool {
	return os.Getenv("AWS_PROFILE") != "" || os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("AWS_SECRET_ACCESS_KEY") != ""
}
