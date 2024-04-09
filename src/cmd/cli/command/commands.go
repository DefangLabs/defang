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
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli"
	cliClient "github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/scope"
	"github.com/defang-io/defang/src/pkg/types"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

const authNeeded = "auth-needed" // annotation to indicate that a command needs authorization
var authNeededAnnotation = map[string]string{authNeeded: ""}

// GLOBALS
var (
	client         cliClient.Client
	cluster        string
	project        *composeTypes.Project
	gitHubClientId = pkg.Getenv("DEFANG_CLIENT_ID", "7b41848ca116eac4b125") // GitHub OAuth app
	hasTty         = cli.IsTerminal && !pkg.GetenvBool("CI")
)

func Execute(ctx context.Context) {
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			cli.Error("Error:", err)
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
			printDefangHint("To manage service secrets, use:", "secret")
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

		FlushAllTracking() // TODO: track errors/panics
		os.Exit(int(code))
	}

	if hasTty && cli.HadWarnings {
		cli.Println(cli.Nop, "For help with warnings, check our FAQ at https://docs.defang.io/docs/faq")
	}

	if hasTty && !pkg.GetenvBool("DEFANG_HIDE_UPDATE") && rand.Intn(10) == 0 {
		if ver, err := GetLatestVersion(ctx); err == nil && semver.Compare(GetCurrentVersion(), ver) < 0 {
			cli.Debug("Latest Version:", ver, "Current Version:", GetCurrentVersion())
			cli.Println(cli.Nop, "A newer version of the CLI is available at https://github.com/defang-io/defang/releases/latest")
			if rand.Intn(10) == 0 && !pkg.GetenvBool("DEFANG_HIDE_HINTS") {
				fmt.Println("To silence these notices, do: export DEFANG_HIDE_UPDATE=1")
			}
		}
	}

}

func SetupCommands() {
	defangFabric := pkg.Getenv("DEFANG_FABRIC", cli.DefaultCluster)
	defangProvider := cliClient.Provider(pkg.Getenv("DEFANG_PROVIDER", "auto"))

	colorMode := ColorAuto
	rootCmd.PersistentFlags().Var(&colorMode, "color", `Colorize output; "auto", "always" or "never"`)
	rootCmd.PersistentFlags().StringVarP(&cluster, "cluster", "s", defangFabric, "Defang cluster to connect to")
	rootCmd.PersistentFlags().VarP(&defangProvider, "provider", "P", `Cloud provider to use; use "aws" for bring-your-own-cloud`)
	rootCmd.PersistentFlags().BoolVarP(&cli.DoVerbose, "verbose", "v", false, "Verbose logging")
	rootCmd.PersistentFlags().BoolVar(&cli.DoDebug, "debug", false, "Debug logging for troubleshooting the CLI")
	rootCmd.PersistentFlags().BoolVar(&cli.DoDryRun, "dry-run", false, "Dry run (don't actually change anything)")
	rootCmd.PersistentFlags().BoolP("non-interactive", "T", !hasTty, "Disable interactive prompts / no TTY")
	rootCmd.PersistentFlags().StringP("cwd", "C", "", "Change directory before running the command")
	rootCmd.PersistentFlags().StringP("file", "f", "*compose.y*ml", `Compose file path`)
	rootCmd.MarkPersistentFlagFilename("file", "yml", "yaml")

	rootCmd.PersistentFlags().StringP("timezone", "z", "Local", "Timezone to use for log timestamps when tailing")
	rootCmd.PersistentFlags().StringP("timeformat", "F", "2006-01-02T15:04:05.000000Z", "Time format to use for log timestamps when tailing")

	// Bootstrap command
	rootCmd.AddCommand(bootstrapCmd)
	bootstrapCmd.AddCommand(bootstrapDestroyCmd)
	bootstrapCmd.AddCommand(bootstrapDownCmd)
	bootstrapCmd.AddCommand(bootstrapRefreshCmd)
	bootstrapCmd.AddCommand(bootstrapTearDownCmd)
	bootstrapCmd.AddCommand(bootstrapListCmd)
	bootstrapCmd.AddCommand(bootstrapCancelCmd)

	// Eula command
	tosCmd.Flags().Bool("agree-tos", false, "Agree to the Defang terms of service")
	rootCmd.AddCommand(tosCmd)

	// Token command
	tokenCmd.Flags().Duration("expires", 24*time.Hour, "Validity duration of the token")
	tokenCmd.Flags().String("scope", "", fmt.Sprintf("Scope of the token; one of %v (required)", scope.All()))
	tokenCmd.MarkFlagRequired("scope")
	rootCmd.AddCommand(tokenCmd)

	// Login Command
	// loginCmd.Flags().Bool("skip-prompt", false, "Skip the login prompt if already logged in"); TODO: Implement this
	rootCmd.AddCommand(loginCmd)

	// Whoami Command
	rootCmd.AddCommand(whoamiCmd)

	// Logout Command
	rootCmd.AddCommand(logoutCmd)

	// Generate Command
	//generateCmd.Flags().StringP("name", "n", "service1", "Name of the service")
	rootCmd.AddCommand(generateCmd)

	// Get Services Command
	getServicesCmd.Flags().BoolP("long", "l", false, "Show more details")
	rootCmd.AddCommand(getServicesCmd)

	// Get Status Command
	rootCmd.AddCommand(getVersionCmd)

	// Secrets Command
	secretsSetCmd.Flags().BoolP("name", "n", false, "Name of the secret (backwards compat)")
	secretsSetCmd.Flags().MarkHidden("name")
	secretsCmd.AddCommand(secretsSetCmd)

	secretsDeleteCmd.Flags().BoolP("name", "n", false, "Name of the secret(s) (backwards compat)")
	secretsDeleteCmd.Flags().MarkHidden("name")
	secretsCmd.AddCommand(secretsDeleteCmd)

	secretsCmd.AddCommand(secretsListCmd)

	rootCmd.AddCommand(secretsCmd)
	rootCmd.AddCommand(restartCmd)

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
	rootCmd.AddCommand(composeCmd)
	composeCmd.AddCommand(composeRestartCmd)
	composeCmd.AddCommand(composeStopCmd)

	// Tail Command
	tailCmd.Flags().StringP("name", "n", "", "Name of the service")
	tailCmd.Flags().String("etag", "", "ETag or deployment ID of the service")
	tailCmd.Flags().BoolP("raw", "r", false, "Show raw (unparsed) logs")
	tailCmd.Flags().String("since", "5s", "Show logs since duration/time")
	rootCmd.AddCommand(tailCmd)

	// Delete Command
	deleteCmd.Flags().BoolP("name", "n", false, "Name of the service(s) (backwards compat)")
	deleteCmd.Flags().MarkHidden("name")
	deleteCmd.Flags().Bool("tail", false, "Tail the service logs after deleting")
	rootCmd.AddCommand(deleteCmd)

	// Send Command
	sendCmd.Flags().StringP("subject", "n", "", "Subject to send the message to (required)")
	sendCmd.Flags().StringP("type", "t", "", "Type of message to send (required)")
	sendCmd.Flags().String("id", "", "ID of the message")
	sendCmd.Flags().StringP("data", "d", "", "String data to send")
	sendCmd.Flags().StringP("content-type", "c", "", "Content-Type of the data")
	sendCmd.MarkFlagRequired("subject")
	sendCmd.MarkFlagRequired("type")
	rootCmd.AddCommand(sendCmd)

	if cli.CanColor {
		// Add some emphasis to the help command
		re := regexp.MustCompile(`(?m)^[A-Za-z ]+?:`)
		templ := re.ReplaceAllString(rootCmd.UsageTemplate(), "\033[1m$0\033[0m")
		rootCmd.SetUsageTemplate(templ)
	}

	origHelpFunc := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		trackCmd(cmd, "Help", P{"args", args})
		origHelpFunc(cmd, args)
	})

}

var rootCmd = &cobra.Command{
	SilenceUsage:  true,
	SilenceErrors: true,
	Use:           "defang",
	Args:          cobra.NoArgs,
	Short:         "Defang CLI manages services on the Defang cluster",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
		provider := cmd.Flag("provider").Value.(*cliClient.Provider)

		// Use "defer" to track any errors that occur during the command
		defer func() {
			trackCmd(cmd, "Invoked", P{"args", args}, P{"err", err}, P{"non-interactive", nonInteractive}, P{"provider", *provider})
		}()

		// Do this first, since any errors will be printed to the console
		color := cmd.Flag("color").Value.(*ColorMode)
		switch *color {
		case ColorAlways:
			cli.ForceColor(true)
		case ColorNever:
			cli.ForceColor(false)
		}

		switch *provider {
		case cliClient.ProviderAuto:
			if awsInEnv() {
				*provider = cliClient.ProviderAWS
			} else {
				*provider = cliClient.ProviderDefang
			}
		case cliClient.ProviderAWS:
			if !awsInEnv() {
				cli.Warn(" ! AWS provider was selected, but AWS environment variables are not set")
			}
		case cliClient.ProviderDefang:
			if awsInEnv() {
				cli.Warn(" ! Using Defang provider, but AWS environment variables were detected; use --provider")
			}
		}

		cd, _ := cmd.Flags().GetString("cwd")
		if cd != "" {
			// Change directory before running the command
			if err = os.Chdir(cd); err != nil {
				return err
			}
		}

		filePath, _ := cmd.InheritedFlags().GetString("file")

		projectName := os.Getenv("COMPOSE_PROJECT_NAME") // overrides the project name, except in the playground env
		if projectName != "" {
			project, err = cli.LoadComposeWithProjectName(filePath, projectName)
		} else {
			tenantID := cli.GetTenantID(cluster)
			projectName = string(tenantID)
			if *provider == cliClient.ProviderDefang {
				project, err = cli.LoadComposeWithProjectName(filePath, projectName) // playground env
			} else {
				project, err = cli.LoadCompose(filePath, tenantID) // fallback to tenant ID
			}
		}

		if err != nil {
			cli.Debug(" - Could not load docker compose file: ", err)
		} else {
			projectName = project.Name
		}

		client = cli.NewClient(cluster, projectName, *provider)

		// Check if we are correctly logged in, but only if the command needs authorization
		if _, ok := cmd.Annotations[authNeeded]; !ok {
			return nil
		}

		if err = client.CheckLogin(cmd.Context()); err != nil {
			if nonInteractive {
				return err
			}
			// Login interactively now; only do this for authorization-related errors
			if connect.CodeOf(err) == connect.CodeUnauthenticated {
				cli.Warn(" !", err)

				if err = cli.InteractiveLogin(cmd.Context(), client, gitHubClientId, cluster); err != nil {
					return err
				}

				// FIXME: the new login might have changed the tenant, so we should reload the project
				client = cli.NewClient(cluster, projectName, *provider) // reconnect with the new token
				if err = client.CheckLogin(cmd.Context()); err == nil { // recheck (new token = new user)
					return nil // success
				}
			}

			// Check if the user has agreed to the terms of service and show a prompt if needed
			if connect.CodeOf(err) == connect.CodeFailedPrecondition {
				cli.Warn(" !", err)
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
		nonInteractive, _ := cmd.Flags().GetBool("non-interactive")

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
		err := cli.Whoami(cmd.Context(), client)
		if err != nil {
			return err
		}
		return nil
	},
}

var generateCmd = &cobra.Command{
	Use:         "generate",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Aliases:     []string{"gen", "new", "init"},
	Short:       "Generate a sample Defang project in the current folder",
	RunE: func(cmd *cobra.Command, args []string) error {
		nonInteractive, _ := cmd.Flags().GetBool("non-interactive")

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
			cli.Warn(" ! The folder is not empty. Files may be overwritten. Press Ctrl+C to abort.")
		}

		cli.Info(" * Working on it. This may take 1 or 2 minutes...")
		_, err = cli.Generate(cmd.Context(), client, prompt.Language, prompt.Description)
		if err != nil {
			return err
		}

		cli.Info(" * Code generated successfully in folder", prompt.Folder)

		// TODO: should we use EDITOR env var instead?
		cmdd := exec.Command("code", ".")
		err = cmdd.Start()
		if err != nil {
			cli.Debug(" - unable to launch VS Code:", err)
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
		cli.Print(cli.BrightCyan, "Defang CLI:    ")
		fmt.Println(GetCurrentVersion())

		cli.Print(cli.BrightCyan, "Latest CLI:    ")
		ver, err := GetLatestVersion(cmd.Context())
		fmt.Println(ver)

		cli.Print(cli.BrightCyan, "Defang Fabric: ")
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

		tz, tf, err := getTimezoneAndFormat(cmd)
		if err != nil {
			return err
		}

		ts = ts.UTC()
		cli.Info(" * Showing logs since", ts.Format(time.RFC3339Nano), "; press Ctrl+C to stop:")
		return cli.Tail(cmd.Context(), client, cli.LogDisplayArgs{
			Service:    name,
			Etag:       etag,
			Since:      ts,
			Raw:        raw,
			TimeZone:   tz,
			TimeFormat: tf,
		})
	},
}

var secretsCmd = &cobra.Command{
	Use:     "secret", // like Docker
	Args:    cobra.NoArgs,
	Aliases: []string{"secrets"},
	Short:   "Add, update, or delete service secrets",
}

var secretsSetCmd = &cobra.Command{
	Use:         "create SECRET", // like Docker
	Annotations: authNeededAnnotation,
	Args:        cobra.ExactArgs(1),
	Aliases:     []string{"set", "add", "put"},
	Short:       "Adds or updates a secret",
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		nonInteractive, _ := cmd.Flags().GetBool("non-interactive")

		var secret string
		if !nonInteractive {
			// Prompt for secret value
			var secretPrompt = &survey.Password{
				Message: fmt.Sprintf("Enter value for secret %q:", name),
				Help:    "The value will be stored securely and cannot be retrieved later.",
			}

			err := survey.AskOne(secretPrompt, &secret)
			if err != nil {
				return err
			}
		} else {
			bytes, err := io.ReadAll(os.Stdin)
			if err != nil && err != io.EOF {
				return fmt.Errorf("failed reading the secret from non-terminal: %w", err)
			}
			secret = strings.TrimSuffix(string(bytes), "\n")
		}

		if err := cli.SecretsSet(cmd.Context(), client, name, secret); err != nil {
			return err
		}
		cli.Info(" * Updated secret value for", name)

		printDefangHint("To update the deployed values, do:", "compose start")
		return nil
	},
}

var secretsDeleteCmd = &cobra.Command{
	Use:         "rm SECRET...", // like Docker
	Annotations: authNeededAnnotation,
	Args:        cobra.MinimumNArgs(1),
	Aliases:     []string{"del", "delete", "remove"},
	Short:       "Deletes one or more secrets",
	RunE: func(cmd *cobra.Command, names []string) error {
		if err := cli.SecretsDelete(cmd.Context(), client, names...); err != nil {
			// Show a warning (not an error) if the secret was not found
			if connect.CodeOf(err) == connect.CodeNotFound {
				cli.Warn(" !", err)
				return nil
			}
			return err
		}
		cli.Info(" * Deleted secret", names)

		printDefangHint("To list the secrets (but not their values), do:", "secret ls")
		return nil
	},
}

var secretsListCmd = &cobra.Command{
	Use:         "ls", // like Docker
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Aliases:     []string{"list"},
	Short:       "List secrets",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.SecretsList(cmd.Context(), client)
	},
}

var composeCmd = &cobra.Command{
	Use:     "compose",
	Aliases: []string{"stack"},
	Args:    cobra.NoArgs,
	Short:   "Work with local Compose files",
}

func printEndpoints(serviceInfos []*v1.ServiceInfo) {
	for _, serviceInfo := range serviceInfos {
		andEndpoints := ""
		if len(serviceInfo.Endpoints) > 0 {
			andEndpoints = "and will be available at:"
		}
		cli.Info(" * Service", serviceInfo.Service.Name, "is in state", serviceInfo.Status, andEndpoints)
		for i, endpoint := range serviceInfo.Endpoints {
			if serviceInfo.Service.Ports[i].Mode == v1.Mode_INGRESS {
				endpoint = "https://" + endpoint
			}
			fmt.Println("   -", endpoint)
		}
		if serviceInfo.Service.Domainname != "" && serviceInfo.ZoneId != "" {
			fmt.Println("   -", "https://"+serviceInfo.Service.Domainname)
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

		tz, tf, err := getTimezoneAndFormat(cmd)
		if err != nil {
			return err
		}

		since := time.Now()
		deploy, err := cli.ComposeStart(cmd.Context(), client, project, force)
		if err != nil {
			return err
		}

		printEndpoints(deploy.Services)

		if detach {
			return nil
		}

		etag := deploy.Etag
		services := "all services"
		if etag != "" {
			services = "deployment ID " + etag
		}

		cli.Info(" * Tailing logs for", services, "; press Ctrl+C to detach:")
		err = cli.Tail(cmd.Context(), client, cli.LogDisplayArgs{
			Service:    "",
			Etag:       etag,
			Since:      since,
			Raw:        false,
			TimeZone:   tz,
			TimeFormat: tf,
		})
		if err != nil {
			return err
		}
		cli.Info(" * Done.")
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

		deploy, err := cli.ComposeStart(cmd.Context(), client, project, force)
		if err != nil {
			return err
		}

		printEndpoints(deploy.Services)

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
		return cli.ComposeRestart(cmd.Context(), client, project)
	},
}

var composeStopCmd = &cobra.Command{
	Use:         "stop",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Reads a Compose file and stops its services",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := cli.ComposeStop(cmd.Context(), client, project)
		if err != nil {
			return err
		}
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

		tz, tf, err := getTimezoneAndFormat(cmd)
		if err != nil {
			return err
		}

		since := time.Now()
		etag, err := cli.ComposeDown(cmd.Context(), client)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				// Show a warning (not an error) if the service was not found
				cli.Warn(" !", err)
				return nil
			}
			return err
		}

		cli.Info(" * Deleted services, deployment ID", etag)

		if detach {
			printDefangHint("To track the update, do:", "tail --etag "+etag)
			return nil
		}

		err = cli.Tail(cmd.Context(), client, cli.LogDisplayArgs{
			Service:    "",
			Etag:       etag,
			Since:      since,
			Raw:        false,
			TimeZone:   tz,
			TimeFormat: tf,
		})
		if err != nil {
			return err
		}
		cli.Info(" * Done.")
		return nil

	},
}

var composeConfigCmd = &cobra.Command{
	Use:   "config",
	Args:  cobra.NoArgs, // TODO: takes optional list of service names
	Short: "Reads a Compose file and shows the generated config",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli.DoDryRun = true                                               // config is like start in a dry run
		_, err := cli.ComposeStart(cmd.Context(), client, project, false) // force=false to calculate the digest
		if !errors.Is(err, cli.ErrDryRun) {
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

		tz, tf, err := getTimezoneAndFormat(cmd)
		if err != nil {
			return err
		}

		since := time.Now()
		etag, err := cli.Delete(cmd.Context(), client, names...)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				// Show a warning (not an error) if the service was not found
				cli.Warn(" !", err)
				return nil
			}
			return err
		}

		cli.Info(" * Deleted service", names, "with deployment ID", etag)

		if !tail {
			printDefangHint("To track the update, do:", "tail --etag "+etag)
			return nil
		}

		cli.Info(" * Tailing logs for update; press Ctrl+C to detach:")
		return cli.Tail(cmd.Context(), client, cli.LogDisplayArgs{
			Service:    "",
			Etag:       etag,
			Since:      since,
			Raw:        false,
			TimeZone:   tz,
			TimeFormat: tf,
		})
	},
}

var restartCmd = &cobra.Command{
	Use:         "restart SERVICE...",
	Annotations: authNeededAnnotation,
	Args:        cobra.MinimumNArgs(1),
	Short:       "Restart one or more services",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.Restart(cmd.Context(), client, args...)
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
		cli.Info(" * Successfully logged out")
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
		tz, tf, err := getTimezoneAndFormat(cmd)
		if err != nil {
			return err
		}
		return cli.BootstrapCommand(cmd.Context(), client, "destroy", tz, tf)
	},
}

var bootstrapDownCmd = &cobra.Command{
	Use:   "down",
	Args:  cobra.NoArgs,
	Short: "Refresh and then destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		tz, tf, err := getTimezoneAndFormat(cmd)
		if err != nil {
			return err
		}
		return cli.BootstrapCommand(cmd.Context(), client, "down", tz, tf)
	},
}

var bootstrapRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Args:  cobra.NoArgs,
	Short: "Refresh the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		tz, tf, err := getTimezoneAndFormat(cmd)
		if err != nil {
			return err
		}
		return cli.BootstrapCommand(cmd.Context(), client, "refresh", tz, tf)
	},
}

var bootstrapTearDownCmd = &cobra.Command{
	Use:   "teardown",
	Args:  cobra.NoArgs,
	Short: "Destroy the CD cluster without destroying the services",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli.Warn(` ! Deleting the CD cluster; this does not delete the services!`)
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
		tz, tf, err := getTimezoneAndFormat(cmd)
		if err != nil {
			return err
		}
		return cli.BootstrapCommand(cmd.Context(), client, "cancel", tz, tf)
	},
}

var tosCmd = &cobra.Command{
	Use:         "terms",
	Aliases:     []string{"tos", "eula", "tac", "tou"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Short:       "Read and/or agree the Defang terms of service",
	RunE: func(cmd *cobra.Command, args []string) error {
		agree, _ := cmd.Flags().GetBool("agree-tos")
		nonInteractive, _ := cmd.Flags().GetBool("non-interactive")

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

func getTimezoneAndFormat(cmd *cobra.Command) (*time.Location, string, error) {
	var timezoneStr, _ = cmd.InheritedFlags().GetString("timezone")
	var tf, _ = cmd.InheritedFlags().GetString("timeformat")

	tz, err := time.LoadLocation(timezoneStr)
	if err != nil {
		return nil, "", fmt.Errorf("invalid timezone: %w", err)
	}

	return tz, tf, nil
}
