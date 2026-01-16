package command

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg"
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
	Use:         "create CONFIG [file|-] | CONFIG=VALUE...", // like Docker
	Annotations: authNeededAnnotation,
	Args:        cobra.MinimumNArgs(0), // Allow 0 args when using --env-file, or multiple configs
	Aliases:     []string{"set", "add", "put"},
	Short:       "Adds or updates a sensitive config value",
	RunE: func(cmd *cobra.Command, args []string) error {
		fromEnv, _ := cmd.Flags().GetBool("env")
		random, _ := cmd.Flags().GetBool("random")
		envFile, _ := cmd.Flags().GetString("env-file")

		// Early validation for multiple configs
		// Distinguish between multiple configs (KEY1=value1 KEY2=value2) and single config with file (CONFIG file)
		isMultipleConfigs := false
		if len(args) > 1 {
			// If the first arg contains '=', it's multiple configs
			// If the first arg doesn't contain '=', it's single config with file (CONFIG file)
			if strings.Contains(args[0], "=") {
				isMultipleConfigs = true

				// Validate: all args must be in KEY=VALUE format
				for _, arg := range args {
					if !strings.Contains(arg, "=") {
						return errors.New("when setting multiple configs, all must be in KEY=VALUE format")
					}
				}

				// Validate: --random is not allowed with multiple configs
				if random {
					return errors.New("--random is only allowed when setting a single config")
				}

				// Validate: --env is not allowed with multiple configs
				if fromEnv {
					return errors.New("--env is only allowed when setting a single config")
				}
			}
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

				if err := cli.ConfigSet(cmd.Context(), projectName, session.Provider, name, value); err != nil {
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

		// Validate args
		if len(args) == 0 {
			return errors.New("CONFIG argument is required when not using --env-file")
		}

		// Handle multiple configs case
		if isMultipleConfigs {
			// Set each config from args
			successCount := 0
			for _, arg := range args {
				parts := strings.SplitN(arg, "=", 2)
				name := parts[0]
				value := parts[1]

				if !pkg.IsValidSecretName(name) {
					term.Warnf("Skipping invalid config name: %q", name)
					continue
				}

				if err := cli.ConfigSet(cmd.Context(), projectName, session.Provider, name, value); err != nil {
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

		// Single config logic
		parts := strings.SplitN(args[0], "=", 2)
		name := parts[0]

		if !pkg.IsValidSecretName(name) {
			return fmt.Errorf("invalid config name: %q", name)
		}

		var value string
		if fromEnv {
			if len(args) > 1 || len(parts) == 2 {
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

		if err := cli.ConfigSet(cmd.Context(), projectName, session.Provider, name, value); err != nil {
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
