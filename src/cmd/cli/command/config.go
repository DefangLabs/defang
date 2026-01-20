package command

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:     "config", // like Docker
	Args:    cobra.NoArgs,
	Aliases: []string{"secrets", "secret"},
	Short:   "Add, update, or delete service config",
}

var configSetCmd = &cobra.Command{
	Use:         "create CONFIG [file|-] | CONFIG...", // like Docker
	Annotations: authNeededAnnotation,
	Args:        cobra.MinimumNArgs(0), // Allow 0 args when using --env-file, or multiple configs
	Aliases:     []string{"set", "add", "put"},
	Short:       "Adds or updates a sensitive config value",
	RunE: func(cmd *cobra.Command, args []string) error {
		fromEnv, _ := cmd.Flags().GetBool("env")
		random, _ := cmd.Flags().GetBool("random")
		envFile, _ := cmd.Flags().GetString("env-file")

		// This command has several modes of operation:
		// 1. Set one or more config(s):
		//  a. from command line args: defang config create CONFIG1=value1 [CONFIG2=value2...]
		//  b. from env: defang config create --env CONFIG1 [CONFIG2...]
		//  c. from random value(s): defang config create --random CONFIG1 [CONFIG2...]
		//  d. from env-file: defang config create --env-file FILE
		//  e. Set specified config(s) from env file: defang config create --env-file FILE CONFIG1 [CONFIG2...]
		// 2. Set one config:
		//  a. from file or stdin: defang config create CONFIG file|-
		//  b. interactively: defang config create CONFIG

		if random && fromEnv {
			return errors.New("cannot use --random with --env")
		}
		if random && envFile != "" {
			return errors.New("cannot use --random with --env-file")
		}
		if fromEnv && envFile != "" {
			return errors.New("cannot use --env with --env-file")
		}
		if len(args) == 0 && envFile == "" {
			return errors.New("provide CONFIG argument or use --env-file to read from a file")
		}
		fromArgs := len(args) > 0 && strings.Contains(args[0], "=")
		if !random && !fromEnv && envFile == "" && !fromArgs && len(args) > 2 {
			return errors.New("too many arguments; provide a single CONFIG or use --env, --random, or --env-file")
		}

		// Make sure we have a project to set config for before asking for a value
		session, err := newCommandSession(cmd)
		if err != nil {
			return err
		}

		projectName, err := client.LoadProjectNameWithFallback(cmd.Context(), session.Loader, session.Provider)
		if err != nil {
			return err
		}

		var envMap map[string]string
		if fromArgs {
			// 1a. Handle CONFIG=VALUE args
			envMap = make(map[string]string)
			for _, pair := range args {
				name, value, found := strings.Cut(pair, "=")
				if !found {
					return errors.New("when setting multiple configs, all must be in KEY=VALUE format")
				}
				envMap[name] = value
			}
		} else if fromEnv {
			// 1b. Handle --env flag: read specified configs from environment
			envMap = make(map[string]string)
			for _, name := range args {
				if value, ok := os.LookupEnv(name); !ok {
					return fmt.Errorf("environment variable %q not found", name)
				} else {
					envMap[name] = value
				}
			}
		} else if random {
			// 1c. Handle --random flag: generate random values for specified configs
			envMap = make(map[string]string)
			for _, name := range args {
				envMap[name] = cli.CreateRandomConfigValue()
			}
		} else if envFile != "" {
			// 1d. Handle --env-file flag: read all or specified configs from the file
			envMap, err = godotenv.Read(envFile)
			if err != nil {
				return fmt.Errorf("failed to read env file %q: %w", envFile, err)
			}

			if len(envMap) == 0 {
				return errors.New("no config found in env file") // or warn?
			}

			if len(args) > 0 {
				// 1e. Set specified config(s) from env file: defang config create --env-file FILE CONFIG1 [CONFIG2...]
				filteredEnvMap := make(map[string]string)
				for _, name := range args {
					if value, ok := envMap[name]; !ok {
						return fmt.Errorf("config %q not found in env file", name)
					} else {
						filteredEnvMap[name] = value
					}
				}
				envMap = filteredEnvMap
			}
		} else if name := args[0]; global.NonInteractive || len(args) == 2 {
			// 2a. Read the value from a file or stdin
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
			// Trim the LF or CRLF at the end because single line values are common
			value := strings.TrimRight(string(bytes), "\r\n")
			envMap = map[string]string{name: value}
		} else {
			// 2b. Prompt for sensitive value
			var sensitivePrompt = &survey.Password{
				Message: fmt.Sprintf("Enter value for %q:", name),
				Help:    "The value will be stored securely and cannot be retrieved later.",
			}

			var value string
			err := survey.AskOne(sensitivePrompt, &value, survey.WithStdio(term.DefaultTerm.Stdio()))
			if err != nil {
				return err
			}
			envMap = map[string]string{name: value}
		}

		var errs []error
		for name, value := range envMap {
			if err := cli.ConfigSet(cmd.Context(), projectName, session.Provider, name, value); err != nil {
				errs = append(errs, err)
			} else {
				term.Info("Updated value for", name)
			}
		}

		term.Infof("Successfully set %d config value(s)", len(envMap)-len(errs))

		printDefangHint("To update the deployed values, do:", "compose up")
		return errors.Join(errs...)
	},
}

var configDeleteCmd = &cobra.Command{
	Use:         "rm CONFIG...", // like Docker
	Annotations: authNeededAnnotation,
	Args:        cobra.MinimumNArgs(1),
	Aliases:     []string{"del", "delete", "remove"},
	Short:       "Removes one or more config values",
	RunE: func(cmd *cobra.Command, names []string) error {
		session, err := newCommandSession(cmd)
		if err != nil {
			return err
		}

		projectName, err := client.LoadProjectNameWithFallback(cmd.Context(), session.Loader, session.Provider)
		if err != nil {
			return err
		}

		if err := cli.ConfigDelete(cmd.Context(), projectName, session.Provider, names...); err != nil {
			// Show a warning (not an error) if the config was not found
			if connect.CodeOf(err) == connect.CodeNotFound {
				term.Warn(client.PrettyError(err))
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
		ctx := cmd.Context()
		session, err := newCommandSession(cmd)
		if err != nil {
			return err
		}
		projectName, err := client.LoadProjectNameWithFallback(ctx, session.Loader, session.Provider)
		if err != nil {
			return err
		}

		return cli.ConfigList(ctx, projectName, session.Provider)
	},
}

var configResolveCmd = &cobra.Command{
	Use:         "resolve",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Aliases:     []string{"final"},
	Short:       "Show the final resolved environment for the project",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		session, err := newCommandSession(cmd)
		if err != nil {
			return err
		}

		project, err := session.Loader.LoadProject(ctx)
		if err != nil {
			return err
		}

		return cli.PrintConfigSummaryAndValidate(ctx, session.Provider, project)
	},
}
