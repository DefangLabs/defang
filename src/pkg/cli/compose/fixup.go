package compose

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/types"
)

func FixupServices(ctx context.Context, provider client.Provider, project *types.Project, upload UploadMode) error {
	// Preload the current config so we can detect which environment variables should be passed as "secrets"
	config, err := provider.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: project.Name})
	if err != nil {
		term.Debugf("failed to load config: %v", err)
		config = &defangv1.Secrets{}
	}
	slices.Sort(config.Names) // sort for binary search

	for _, svccfg := range project.Services {
		// Fixup ports (which affects service name replacement by ReplaceServiceNameWithDNS)
		for i, port := range svccfg.Ports {
			fixupPort(&port)
			svccfg.Ports[i] = port
		}
	}

	// Fixup any pseudo services (this might create port configs, which will affect service name replacement by ReplaceServiceNameWithDNS)
	for _, svccfg := range project.Services {
		_, managedRedis := svccfg.Extensions["x-defang-redis"]
		if managedRedis {
			if err := fixupRedisService(&svccfg, provider); err != nil {
				return fmt.Errorf("service %q: %w", svccfg.Name, err)
			}
		}

		_, managedPostgres := svccfg.Extensions["x-defang-postgres"]
		if managedPostgres {
			if err := fixupPostgresService(&svccfg, provider); err != nil {
				return fmt.Errorf("service %q: %w", svccfg.Name, err)
			}
		}

		_, managedMongo := svccfg.Extensions["x-defang-mongodb"]
		if managedMongo {
			if err := fixupMongoService(&svccfg, provider); err != nil {
				return fmt.Errorf("service %q: %w", svccfg.Name, err)
			}
		}

		if svccfg.Provider != nil && svccfg.Provider.Type == "model" && svccfg.Image == "" && svccfg.Build == nil {
			fixupModelProvider(&svccfg, project)
		}

		if _, llm := svccfg.Extensions["x-defang-llm"]; llm {
			fixupLLM(&svccfg)
		}

		// update the concrete service with the fixed up object
		project.Services[svccfg.Name] = svccfg
	}

	svcNameReplacer := NewServiceNameReplacer(provider, project)

	for _, svccfg := range project.Services {
		// Upload the build context, if any; TODO: parallelize
		if svccfg.Build != nil {
			// Pack the build context into a tarball and upload
			url, err := getRemoteBuildContext(ctx, provider, project.Name, svccfg.Name, svccfg.Build, upload)
			if err != nil {
				return err
			}
			svccfg.Build.Context = url

			removedArgs := []string{}
			for key, value := range svccfg.Build.Args {
				if key == "" || value == nil {
					removedArgs = append(removedArgs, key)
					delete(svccfg.Build.Args, key) // remove the empty key; this is safe
					continue
				}

				val := svcNameReplacer.ReplaceServiceNameWithDNS(svccfg.Name, key, *value, BuildArgs)
				svccfg.Build.Args[key] = &val
			}

			if len(removedArgs) > 0 {
				term.Warnf("service %q: skipping unset build argument %q", svccfg.Name, removedArgs)
			}
		}

		// Fixup secret references; secrets are supposed to be files, not env, but it's kept for backward compatibility
		for i, secret := range svccfg.Secrets {
			if i == 0 { // only warn once
				term.Warnf("service %q: secrets will be exposed as environment variables, not files (use 'environment' instead)", svccfg.Name)
			}
			svccfg.Environment[secret.Source] = nil
		}
		svccfg.Secrets = nil

		// Fixup environment variables
		shownOnce := false
		useCfg := []string{}
		overriddenCfg := []string{}
		for key, value := range svccfg.Environment {
			// A bug in Compose-go env file parsing can cause empty keys
			if key == "" {
				if !shownOnce {
					term.Warnf("service %q: skipping unset environment variable key", svccfg.Name)
					shownOnce = true
				}
				delete(svccfg.Environment, key) // remove the empty key; this is safe
				continue
			}
			if value == nil {
				continue // will be from config
			}

			// Check if the environment variable is an existing config; if so, mark it as such
			if _, ok := slices.BinarySearch(config.Names, key); ok {
				if svcNameReplacer.HasServiceName(*value) {
					useCfg = append(useCfg, key)
				} else {
					overriddenCfg = append(overriddenCfg, key)
				}
				svccfg.Environment[key] = nil
				continue
			}

			val := svcNameReplacer.ReplaceServiceNameWithDNS(svccfg.Name, key, *value, EnvironmentVars)
			svccfg.Environment[key] = &val
		}

		if len(useCfg) > 0 {
			term.Warnf("service %q: environment variable(s) %q will use the `defang config` value instead of adjusted service name", svccfg.Name, useCfg)
		}

		if len(overriddenCfg) > 0 {
			term.Warnf("service %q: environment variable(s) %q overridden by config", svccfg.Name, overriddenCfg)
		}

		_, scaling := svccfg.Extensions["x-defang-autoscaling"]
		if scaling {
			if _, ok := provider.(*client.PlaygroundProvider); ok {
				term.Warnf("service %q: auto-scaling is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name)
			}
		}

		// update the concrete service with the fixed up object
		project.Services[svccfg.Name] = svccfg
	}

	return nil
}

func parsePortString(port string) (uint32, error) {
	if p, err := strconv.ParseUint(port, 10, 16); err != nil {
		return 0, fmt.Errorf("invalid port number %q: %w", port, err)
	} else {
		return uint32(p), nil
	}
}

func fixupLLM(svccfg *types.ServiceConfig) {
	image := getImageRepo(svccfg.Image)
	if strings.HasSuffix(image, "/openai-access-gateway") && len(svccfg.Ports) == 0 {
		// HACK: we must have at least one host port to get a CNAME for the service
		var port uint32 = 80
		term.Debugf("service %q: adding LLM host port %d", svccfg.Name, port)
		svccfg.Ports = []types.ServicePortConfig{{Target: port, Mode: Mode_HOST, Protocol: Protocol_TCP}}
	}
}

func fixupPostgresService(svccfg *types.ServiceConfig, provider client.Provider) error {
	if _, ok := provider.(*client.PlaygroundProvider); ok {
		term.Warnf("service %q: managed postgres is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name)
		delete(svccfg.Extensions, "x-defang-postgres")
	} else if len(svccfg.Ports) == 0 {
		// HACK: we must have at least one host port to get a CNAME for the service
		var port uint32 = 5432
		// Check PGPORT environment variable for port number https://www.postgresql.org/docs/current/libpq-envars.html
		if pgport := svccfg.Environment["PGPORT"]; pgport != nil {
			var err error
			port, err = parsePortString(*pgport)
			if err != nil {
				return err
			}
		}
		term.Debugf("service %q: adding postgres host port %d", svccfg.Name, port)
		svccfg.Ports = []types.ServicePortConfig{{Target: port, Mode: Mode_HOST, Protocol: Protocol_TCP}}
	}
	return nil
}

func fixupMongoService(svccfg *types.ServiceConfig, provider client.Provider) error {
	if _, ok := provider.(*client.PlaygroundProvider); ok {
		term.Warnf("service %q: managed mongodb is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name)
		delete(svccfg.Extensions, "x-defang-mongodb")
	}
	if len(svccfg.Ports) == 0 {
		// HACK: we must have at least one host port to get a CNAME for the service
		var port uint32 = 27017
		// Check --port
		args := append(svccfg.Entrypoint, svccfg.Command...)
		for i, arg := range args {
			if arg == "--port" && i+1 < len(args) {
				var err error
				port, err = parsePortString(args[i+1])
				if err != nil {
					return err
				}
			}
		}
		term.Debugf("service %q: adding mongodb host port %d", svccfg.Name, port)
		svccfg.Ports = []types.ServicePortConfig{{Target: port, Mode: Mode_HOST, Protocol: Protocol_TCP}}
	}
	return nil
}

func fixupRedisService(svccfg *types.ServiceConfig, provider client.Provider) error {
	if _, ok := provider.(*client.PlaygroundProvider); ok {
		term.Warnf("service %q: Managed redis is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name)
		delete(svccfg.Extensions, "x-defang-redis")
	} else if len(svccfg.Ports) == 0 {
		// HACK: we must have at least one host port to get a CNAME for the service https://redis.io/docs/latest/operate/oss_and_stack/management/config/
		var port uint32 = 6379
		// Check entrypoint or command for --port argument
		args := append(svccfg.Entrypoint, svccfg.Command...)
		for i, arg := range args {
			if arg == "--port" && i+1 < len(args) {
				var err error
				port, err = parsePortString(args[i+1])
				if err != nil {
					return err
				}
			}
		}
		term.Debugf("service %q: adding redis host port %d", svccfg.Name, port)
		svccfg.Ports = []types.ServicePortConfig{{Target: port, Mode: Mode_HOST, Protocol: Protocol_TCP}}
	}
	return nil
}

func fixupModelProvider(svccfg *types.ServiceConfig, project *types.Project) {
	// Declare a private network for the model provider
	const modelProviderNetwork = "model_provider_private"

	// Local Docker sets [SERVICE]_URL and [SERVICE]_MODEL environment variables on the dependent services
	envName := strings.ToUpper(svccfg.Name) // TODO: handle characters that are not allowed in env vars, like '-'
	urlEnv := envName + "_URL"
	urlVal := "http://" + svccfg.Name + "/api/v1/"
	modelEnv := envName + "_MODEL"
	modelVal := svccfg.Provider.Options["model"]

	empty := ""
	// svccfg.Deploy.Resources.Reservations.Limits = &types.Resources{} TODO: avoid memory limits warning
	svccfg.Environment = types.MappingWithEquals{"OPENAI_API_KEY": &empty} // disable auth; see https://github.com/DefangLabs/openai-access-gateway/pull/5
	// svccfg.HealthCheck = &types.ServiceHealthCheckConfig{} TODO: add healthcheck
	svccfg.Image = "defangio/openai-access-gateway"
	svccfg.Networks[modelProviderNetwork] = nil
	svccfg.Ports = []types.ServicePortConfig{{Target: 80, Mode: Mode_HOST, Protocol: Protocol_TCP}}
	svccfg.Provider = nil              // remove "provider:" because current backend will not accept it
	delete(svccfg.Networks, "default") // remove the default network
	project.Networks[modelProviderNetwork] = types.NetworkConfig{Name: modelProviderNetwork}

	// Set environment variables (url and model) for any service that depends on the provider pseudo service
	for _, dependency := range project.Services {
		if _, ok := dependency.DependsOn[svccfg.Name]; ok {
			if dependency.Environment == nil {
				dependency.Environment = make(types.MappingWithEquals)
			}
			dependency.Networks[modelProviderNetwork] = nil
			if _, ok := dependency.Environment[urlEnv]; !ok {
				dependency.Environment[urlEnv] = &urlVal
			}
			if _, ok := dependency.Environment[modelEnv]; !ok && modelVal != "" {
				dependency.Environment[modelEnv] = &modelVal
			}
		}
	}
}

func getImageRepo(imageRepo string) string {
	image := strings.ToLower(imageRepo)
	image, _, _ = strings.Cut(image, ":")
	return image
}
