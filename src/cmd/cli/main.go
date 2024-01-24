package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
	"golang.org/x/term"

	"github.com/AlecAivazis/survey/v2"

	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli"
	cliClient "github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/scope"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

//
// GLOBALS
//

var (
	client   cliClient.Client
	clientId = pkg.Getenv("DEFANG_CLIENT_ID", "7b41848ca116eac4b125")
	hasTty   = term.IsTerminal(int(os.Stdin.Fd())) && !pkg.GetenvBool("CI")
	tenantId = pkg.DEFAULT_TENANT
	version  = "development" // overwritten by build script -ldflags "-X main.version=..."
)

const autoConnect = "auto-connect" // annotation to indicate that a command needs to connect to the cluster
var autoConnectAnnotation = map[string]string{autoConnect: ""}

const authNeeded = "auth-needed"                                              // annotation to indicate that a command needs authorization
var authNeededAnnotation = map[string]string{authNeeded: "", autoConnect: ""} // auth implies auto-connect

var rootCmd = &cobra.Command{
	SilenceUsage:  true,
	SilenceErrors: true,
	Use:           "defang",
	Args:          cobra.NoArgs,
	Short:         "Defang CLI manages services on the Defang cluster",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cd, _ := cmd.Flags().GetString("cwd")
		if cd != "" {
			if err := os.Chdir(cd); err != nil {
				return err
			}
		}

		color := cmd.Flag("color").Value.(*ColorMode)
		switch *color {
		case ColorAlways:
			cli.DoColor = true
		case ColorNone:
			cli.DoColor = false
		}

		// Not all commands need a connection, so we should only connect when needed
		if _, ok := cmd.Annotations[autoConnect]; !ok {
			return nil
		}

		server := getServer(cmd)

		provider, _ := cmd.Flag("provider").Value.(*cliClient.Provider)
		client, tenantId = cli.Connect(server, *provider)

		// Check if we are correctly logged in, but only if the command needs authorization
		if _, ok := cmd.Annotations[authNeeded]; !ok {
			return nil
		}

		nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
		if err := cli.CheckLogin(cmd.Context(), client); err != nil && !nonInteractive {
			// Login now; only do this for authorization-related errors
			if connect.CodeOf(err) != connect.CodeUnauthenticated {
				return err
			}
			cli.Warn(" !", err)
			if err := cli.LoginAndSaveAccessToken(cmd.Context(), client, clientId, server); err != nil {
				return err
			}
			client, tenantId = cli.Connect(server, *provider) // reconnect with the new token
		}
		return nil
	},
}

func getServer(cmd *cobra.Command) string {
	server, _ := cmd.Flags().GetString("cluster")
	if _, _, err := net.SplitHostPort(server); err != nil {
		server = server + ":443"
	}
	return server
}

var loginCmd = &cobra.Command{
	Use:         "login",
	Annotations: autoConnectAnnotation,
	Args:        cobra.NoArgs,
	Short:       "Authenticate to the Defang cluster",
	RunE: func(cmd *cobra.Command, args []string) error {

		err := cli.LoginAndSaveAccessToken(cmd.Context(), client, clientId, getServer(cmd))
		if err != nil {
			return err
		}

		printDefangHint("To generate a sample service, do:", "generate")
		return nil
	},
}

var whoamiCmd = &cobra.Command{
	Use:         "whoami",
	Annotations: autoConnectAnnotation,
	Args:        cobra.NoArgs,
	Short:       "Show the current user",
	RunE: func(cmd *cobra.Command, args []string) error {
		server := getServer(cmd)
		tenant, err := cli.Whoami(server)
		if err != nil {
			return err
		}
		cli.Info(" * You are logged in as", tenant)
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
		err := cli.GetServices(cmd.Context(), client)
		if err != nil {
			return err
		}
		// printDefangHint("To get more information about a service, do:", "get service <name>")
		return nil
	},
}

var getVersionCmd = &cobra.Command{
	Use:         "version",
	Annotations: autoConnectAnnotation,
	Args:        cobra.NoArgs,
	Aliases:     []string{"ver", "stat", "status"}, // for backwards compatibility
	Short:       "Get version information for the CLI and Fabric service",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli.Print(cli.BrightCyan, "Defang CLI:    ")
		fmt.Println(version)

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

		ts = ts.UTC()
		cli.Info(" * Showing logs since", ts.Format(time.RFC3339Nano), "; press Ctrl+C to stop:")
		return cli.Tail(cmd.Context(), client, name, etag, ts, raw)
	},
}

var secretsCmd = &cobra.Command{
	Use:     "secret",
	Args:    cobra.NoArgs,
	Aliases: []string{"secrets"},
	Short:   "Add, update, or delete service secrets",
}

var secretsSetCmd = &cobra.Command{
	Use:         "set",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Aliases:     []string{"add", "put"},
	Short:       "Adds or updates a secret",
	RunE: func(cmd *cobra.Command, args []string) error {
		var name, _ = cmd.Flags().GetString("name")

		var secret string
		nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
		if !nonInteractive {
			// check if we are properly connected / authenticated before asking the questions
			if err := cli.CheckLogin(cmd.Context(), client); err != nil {
				return err
			}

			// Prompt for secret value
			cli.Print(cli.Bright, "? Enter value for secret '", name, "': ")

			byteSecret, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return err
			}
			fmt.Println()
			secret = string(byteSecret)
		} else {
			reader := bufio.NewReader(os.Stdin)
			s, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				return fmt.Errorf("failed reading the secret from non-terminal: %w", err)
			}
			secret = strings.TrimSuffix(s, "\n")
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
	Use:         "delete",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Aliases:     []string{"del", "rm", "remove"},
	Short:       "Deletes a secret",
	RunE: func(cmd *cobra.Command, args []string) error {
		var name, _ = cmd.Flags().GetString("name")

		if err := cli.SecretsDelete(cmd.Context(), client, name); err != nil {
			// Show a warning (not an error) if the secret was not found
			if connect.CodeOf(err) == connect.CodeNotFound {
				cli.Warn(" !", err)
				return nil
			}
			return err
		}
		cli.Info(" * Deleted secret", name)

		printDefangHint("To list the secrets (but not their values), do:", "secret ls")
		return nil
	},
}

var secretsListCmd = &cobra.Command{
	Use:         "ls",
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

func printEndpoints(serviceInfos []*defangv1.ServiceInfo) {
	for _, serviceInfo := range serviceInfos {
		andEndpoints := ""
		if len(serviceInfo.Endpoints) > 0 {
			andEndpoints = "and will be available at:"
		}
		cli.Info(" * Service", serviceInfo.Service.Name, "is in state", serviceInfo.Status, andEndpoints)
		for i, endpoint := range serviceInfo.Endpoints {
			if serviceInfo.Service.Ports[i].Mode == defangv1.Mode_INGRESS {
				endpoint = "https://" + endpoint
			}
			fmt.Println("   -", endpoint)
		}
	}
}

var composeUpCmd = &cobra.Command{
	Use:         "up",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Like 'start' but immediately tracks the progress of the deployment",
	RunE: func(cmd *cobra.Command, args []string) error {
		var filePath, _ = cmd.InheritedFlags().GetString("file")
		var force, _ = cmd.Flags().GetBool("force")

		since := time.Now()
		serviceInfos, err := cli.ComposeStart(cmd.Context(), client, filePath, string(tenantId), force)
		if err != nil {
			return err
		}

		printEndpoints(serviceInfos)

		var etag string
		services := "all services"
		if len(serviceInfos) == 1 {
			etag = serviceInfos[0].Etag
			services = "deployment ID " + etag
		}

		cli.Info(" * Tailing logs for", services, "; press Ctrl+C to detach:")
		return cli.Tail(cmd.Context(), client, "", etag, since, false)
	},
}

var composeStartCmd = &cobra.Command{
	Use:         "start",
	Aliases:     []string{"deploy"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Reads a Compose file and deploys services to the cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		var filePath, _ = cmd.InheritedFlags().GetString("file")
		var force, _ = cmd.Flags().GetBool("force")

		serviceInfos, err := cli.ComposeStart(cmd.Context(), client, filePath, string(tenantId), force)
		if err != nil {
			return err
		}

		printEndpoints(serviceInfos)

		command := "tail"
		if len(serviceInfos) == 1 {
			command += " --etag " + serviceInfos[0].Etag
		}
		printDefangHint("To track the deployment status, do:", command)
		return nil
	},
}

var composeRestartCmd = &cobra.Command{
	Use:         "restart",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Reads a Compose file and restarts its services",
	RunE: func(cmd *cobra.Command, args []string) error {
		var filePath, _ = cmd.InheritedFlags().GetString("file")

		_, err := cli.ComposeRestart(cmd.Context(), client, filePath, string(tenantId))
		if err != nil {
			return err
		}
		return nil
	},
}

var composeDownCmd = &cobra.Command{
	Use:         "down",
	Aliases:     []string{"stop", "rm"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs, // TODO: takes optional list of service names
	Short:       "Reads a Compose file and deletes services from the cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		var filePath, _ = cmd.InheritedFlags().GetString("file")
		var tail, _ = cmd.Flags().GetBool("tail")

		since := time.Now()
		etag, err := cli.ComposeDown(cmd.Context(), client, filePath, string(tenantId))
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				// Show a warning (not an error) if the service was not found
				cli.Warn(" !", err)
				return nil
			}
			return err
		}

		cli.Info(" * Deleted services, deployment ID", etag)

		if tail {
			cli.Info(" * Tailing logs for deployment; press Ctrl+C to detach:")
			return cli.Tail(cmd.Context(), client, "", etag, since, false)
		}

		printDefangHint("To track the deployment status, do:", "tail --etag "+etag)
		return nil
	},
}

var composeConfigCmd = &cobra.Command{
	Use:   "config",
	Args:  cobra.NoArgs, // TODO: takes optional list of service names
	Short: "Reads a Compose file and shows the generated config",
	RunE: func(cmd *cobra.Command, args []string) error {
		var filePath, _ = cmd.InheritedFlags().GetString("file")

		cli.DoDryRun = true                                                                  // config is like start in a dry run
		_, err := cli.ComposeStart(cmd.Context(), client, filePath, string(tenantId), false) // force=false to calculate the digest
		return err
	},
}

var deleteCmd = &cobra.Command{
	Use:         "delete",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Aliases:     []string{"del", "rm", "remove"},
	Short:       "Delete a service from the cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		var name, _ = cmd.Flags().GetString("name")
		var tail, _ = cmd.Flags().GetBool("tail")

		since := time.Now()
		etag, err := cli.Delete(cmd.Context(), client, name)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				// Show a warning (not an error) if the service was not found
				cli.Warn(" !", err)
				return nil
			}
			return err
		}

		cli.Info(" * Deleted service", name, "with deployment ID", etag)

		if tail {
			cli.Info(" * Tailing logs for deployment; press Ctrl+C to detach:")
			return cli.Tail(cmd.Context(), client, "", etag, since, false)
		}

		printDefangHint("To track the deployment status, do:", "tail --etag "+etag)
		return nil
	},
}

var restartCmd = &cobra.Command{
	Use:         "restart",
	Annotations: authNeededAnnotation,
	Args:        cobra.MinimumNArgs(1),
	Short:       "Restart one or more services",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := cli.Restart(cmd.Context(), client, args...)
		if err != nil {
			return err
		}
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
		return cli.Token(cmd.Context(), client, clientId, pkg.DEFAULT_TENANT, expires, scope.Scope(s))
	},
}

var logoutCmd = &cobra.Command{
	Use:         "logout",
	Annotations: autoConnectAnnotation,
	Args:        cobra.NoArgs,
	Aliases:     []string{"logoff", "revoke"},
	Short:       "Log out",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cli.Logout(cmd.Context(), client); err != nil {
			return err
		}
		cli.Info(" * Successfully logged out")
		return nil
	},
}

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Args:  cobra.NoArgs,
	Short: "Manage a BYOC account",
}

var bootstrapDestroyCmd = &cobra.Command{
	Use:         "destroy",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Short:       "Destroy the bootstrapped CD resources",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli.Warn(` ! This will fail unless you "down" the stacks first!`)
		return cli.BootstrapCommand(cmd.Context(), client, "destroy")
	},
}

var bootstrapDownCmd = &cobra.Command{
	Use:         "down",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Short:       "Destroy the service stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli.Warn(` ! Destroying the cluster`)
		return cli.BootstrapCommand(cmd.Context(), client, "down")
	},
}

var bootstrapRefreshCmd = &cobra.Command{
	Use:         "refresh",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Short:       "Refresh the service resources",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.BootstrapCommand(cmd.Context(), client, "refresh")
	},
}

func main() {
	defangFabric := pkg.Getenv("DEFANG_FABRIC", "fabric-prod1.defang.dev")
	defangProvider := cliClient.Provider(pkg.Getenv("DEFANG_PROVIDER", "auto"))

	colorMode := ColorAuto
	rootCmd.PersistentFlags().Var(&colorMode, "color", `Colorize output; "auto", "always" or "never"`)
	rootCmd.PersistentFlags().StringP("cluster", "s", defangFabric, "Defang cluster to connect to")
	rootCmd.PersistentFlags().VarP(&defangProvider, "provider", "P", `Cloud provider to use; use "aws" for bring-your-own-cloud`)
	rootCmd.PersistentFlags().BoolVarP(&cli.DoVerbose, "verbose", "v", false, "Verbose logging")
	rootCmd.PersistentFlags().BoolVar(&cli.DoDryRun, "dry-run", false, "Dry run (don't actually change anything)")
	rootCmd.PersistentFlags().BoolP("non-interactive", "T", !hasTty, "Disable interactive prompts / no TTY")
	rootCmd.PersistentFlags().StringP("cwd", "C", "", "Change directory before running the command")
	//rootCmd.PersistentFlags().StringVarP(&userLicense, "license", "l", "", "name of license for the project")

	// Bootstrap command
	rootCmd.AddCommand(bootstrapCmd)
	bootstrapCmd.AddCommand(bootstrapDestroyCmd)
	bootstrapCmd.AddCommand(bootstrapDownCmd)
	bootstrapCmd.AddCommand(bootstrapRefreshCmd)

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
	rootCmd.AddCommand(getServicesCmd)

	// Get Status Command
	rootCmd.AddCommand(getVersionCmd)

	// Secrets Command
	secretsSetCmd.Flags().StringP("name", "n", "", "Name of the secret (required)")
	secretsSetCmd.MarkFlagRequired("name")
	secretsCmd.AddCommand(secretsSetCmd)

	secretsDeleteCmd.Flags().StringP("name", "n", "", "Name of the secret (required)")
	secretsDeleteCmd.MarkFlagRequired("name")
	secretsCmd.AddCommand(secretsDeleteCmd)

	secretsCmd.AddCommand(secretsListCmd)

	rootCmd.AddCommand(secretsCmd)
	rootCmd.AddCommand(restartCmd)

	// Compose Command
	composeCmd.PersistentFlags().StringP("file", "f", "*compose.y*ml", `Compose file path`)
	composeCmd.MarkPersistentFlagFilename("file", "yml", "yaml")
	// composeCmd.Flags().Bool("compatibility", false, "Run compose in backward compatibility mode"); TODO: Implement compose option
	// composeCmd.Flags().String("env-file", "", "Specify an alternate environment file."); TODO: Implement compose option
	// composeCmd.Flags().Int("parallel", -1, "Control max parallelism, -1 for unlimited (default -1)"); TODO: Implement compose option
	// composeCmd.Flags().String("profile", "", "Specify a profile to enable"); TODO: Implement compose option
	// composeCmd.Flags().String("project-directory", "", "Specify an alternate working directory"); TODO: Implement compose option
	// composeCmd.Flags().StringP("project", "p", "", "Compose project name"); TODO: Implement compose option
	composeUpCmd.Flags().Bool("tail", false, "Tail the service logs after updating") // obsolete, but keep for backwards compatibility
	composeUpCmd.Flags().Bool("force", false, "Force a build of the image even if nothing has changed")
	composeCmd.AddCommand(composeUpCmd)
	composeCmd.AddCommand(composeConfigCmd)
	composeDownCmd.Flags().Bool("tail", false, "Tail the service logs after deleting")
	composeCmd.AddCommand(composeDownCmd)
	composeStartCmd.Flags().Bool("force", false, "Force a build of the image even if nothing has changed")
	composeCmd.AddCommand(composeStartCmd)
	rootCmd.AddCommand(composeCmd)
	composeCmd.AddCommand(composeRestartCmd)

	// Tail Command
	tailCmd.Flags().StringP("name", "n", "", "Name of the service")
	tailCmd.Flags().String("etag", "", "ETag or deployment ID of the service")
	tailCmd.Flags().BoolP("raw", "r", false, "Show raw (unparsed) logs")
	tailCmd.Flags().String("since", "5s", "Show logs since duration/time")
	rootCmd.AddCommand(tailCmd)

	// Delete Command
	deleteCmd.Flags().StringP("name", "n", "", "Name of the service (required)")
	deleteCmd.Flags().BoolP("tail", "f", false, "Tail the service logs after deleting")
	deleteCmd.MarkFlagRequired("name")
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

	// Handle Ctrl+C so we can exit gracefully
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	defer close(sigs)
	signal.Notify(sigs, os.Interrupt)
	defer signal.Stop(sigs)

	go func() {
		<-sigs
		cancel()
	}()

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
			printDefangHint("Please use the following command to log in:", "login")
		}

		os.Exit(int(code))
	}

	if !pkg.GetenvBool("DEFANG_HIDE_UPDATE") {
		if ver, err := GetLatestVersion(ctx); err == nil && semver.Compare(version, ver) < 0 {
			cli.Warn(" ! A newer version of the CLI is available at https://github.com/defang-io/defang/releases/latest")
		}
	}
}

func prettyExecutable(def string) string {
	if os.Args[0] == def {
		return def
	}
	executable, _ := os.Executable()
	if executable == "" || strings.HasPrefix(executable, os.TempDir()) {
		// If the binary is from the temp folder, default to def
		return def
	}
	wd, err := os.Getwd()
	if err != nil {
		return def
	}
	executable, _ = filepath.Rel(wd, executable)
	if executable == def {
		executable = "./" + def // to ensure it's executable
	}
	if executable == "" {
		return def
	}
	return executable
}

func printDefangHint(hint, args string) {
	if pkg.GetenvBool("DEFANG_HIDE_HINTS") || !hasTty {
		return
	}

	executable := prettyExecutable("defang")

	fmt.Printf("\n%s\n", hint)
	providerFlag := rootCmd.Flag("provider")
	clusterFlag := rootCmd.Flag("cluster")
	if providerFlag.Changed {
		fmt.Printf("\n  %s --provider %s %s\n\n", executable, providerFlag.Value.String(), args)
	} else if clusterFlag.Changed {
		fmt.Printf("\n  %s --cluster %s %s\n\n", executable, clusterFlag.Value.String(), args)
	} else {
		fmt.Printf("\n  %s %s\n\n", executable, args)
	}
	if rand.Intn(10) == 0 {
		fmt.Println("To silence these hints, do: export DEFANG_HIDE_HINTS=1")
	}
}
