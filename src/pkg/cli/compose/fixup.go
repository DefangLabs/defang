package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/types"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
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
			svccfg.Ports[i] = fixupPort(port)
		}
	}

	// Fixup any pseudo services (this might create port configs, which will affect service name replacement by ReplaceServiceNameWithDNS)
	for _, svccfg := range project.Services {
		_, managedRedis := svccfg.Extensions["x-defang-redis"]
		if managedRedis {
			if err := fixupRedisService(&svccfg, provider, upload); err != nil {
				return fmt.Errorf("service %q: %w", svccfg.Name, err)
			}
		}

		_, managedPostgres := svccfg.Extensions["x-defang-postgres"]
		if managedPostgres {
			if err := fixupPostgresService(&svccfg, provider, upload); err != nil {
				return fmt.Errorf("service %q: %w", svccfg.Name, err)
			}
		}

		_, managedMongo := svccfg.Extensions["x-defang-mongodb"]
		if managedMongo {
			if err := fixupMongoService(&svccfg, provider, upload); err != nil {
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
			// Because of normalization, Dockerfile is always set to "Dockerfile" even if it was not specified in the compose file.
			if svccfg.Build.Dockerfile != "" {
				// Check if the dockerfile exists
				dockerfilePath := filepath.Join(svccfg.Build.Context, svccfg.Build.Dockerfile)
				if _, err := os.Stat(dockerfilePath); err != nil {
					// In this case we know that the dockerfile is not in the location the compose file specifies,
					// so can assume that the dockerfile has been normalized to the default "Dockerfile".
					if svccfg.Build.Dockerfile != "Dockerfile" {
						// An explicit Dockerfile was specified, but it does not exist.
						return fmt.Errorf("service %q: dockerfile not found: %w", svccfg.Name, ErrDockerfileNotFound)
					}

					// Undo normalization
					svccfg.Build.Dockerfile = ""
				}
			}

			// Pack the build context into a Archive and upload
			url, err := getRemoteBuildContext(ctx, provider, project.Name, svccfg.Name, svccfg.Build, upload)
			if err != nil {
				return err
			}
			svccfg.Build.Context = url

			var removedArgs []string
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
		var notAdjusted []string
		var overridden []string
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
					notAdjusted = append(notAdjusted, key)
				} else {
					overridden = append(overridden, key)
				}
				svccfg.Environment[key] = nil
				continue
			}

			if upload != UploadModeEstimate {
				val := svcNameReplacer.ReplaceServiceNameWithDNS(svccfg.Name, key, *value, EnvironmentVars)
				svccfg.Environment[key] = &val
			}
		}

		if len(notAdjusted) > 0 {
			term.Warnf("service %q: environment variable(s) %q will use the `defang config` value instead of adjusted service name", svccfg.Name, notAdjusted)
		}

		if len(overridden) > 0 {
			term.Warnf("service %q: environment variable(s) %q overridden by config", svccfg.Name, overridden)
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

func fixupPostgresService(svccfg *types.ServiceConfig, provider client.Provider, upload UploadMode) error {
	if _, ok := provider.(*client.PlaygroundProvider); ok && upload != UploadModeEstimate {
		term.Warnf("service %q: managed postgres is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name)
	}
	if len(svccfg.Ports) == 0 {
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

func fixupMongoService(svccfg *types.ServiceConfig, provider client.Provider, upload UploadMode) error {
	if _, ok := provider.(*client.PlaygroundProvider); ok && upload != UploadModeEstimate {
		term.Warnf("service %q: managed mongodb is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name)
	}
	if len(svccfg.Ports) == 0 {
		// HACK: we must have at least one host port to get a CNAME for the service
		var port uint32 = 27017
		args := append(svccfg.Entrypoint, svccfg.Command...)
		for i, arg := range args {
			if arg == "--shardsvr" {
				port = 27018
				continue // looking for --port
			} else if arg == "--configsvr" {
				port = 27019
				continue // looking for --port
			} else if num, ok := strings.CutPrefix(arg, "--port="); ok {
				arg = num
			} else if arg == "--port" && i+1 < len(args) {
				arg = args[i+1]
			} else {
				continue
			}
			var err error
			port, err = parsePortString(arg)
			if err != nil {
				return err
			}
			break // done
		}
		term.Debugf("service %q: adding mongodb host port %d", svccfg.Name, port)
		svccfg.Ports = []types.ServicePortConfig{{Target: port, Mode: Mode_HOST, Protocol: Protocol_TCP}}
	}
	return nil
}

func fixupRedisService(svccfg *types.ServiceConfig, provider client.Provider, upload UploadMode) error {
	if _, ok := provider.(*client.PlaygroundProvider); ok && upload != UploadModeEstimate {
		term.Warnf("service %q: Managed redis is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name)
	}
	if len(svccfg.Ports) == 0 {
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
				// continue; last one wins
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
	modelEnvKey := envName + "_MODEL"
	modelVals := svccfg.Provider.Options["model"]

	empty := ""
	// svccfg.Deploy.Resources.Reservations.Limits = &types.Resources{} TODO: avoid memory limits warning
	if svccfg.Environment == nil {
		svccfg.Environment = types.MappingWithEquals{}
	}
	if _, exists := svccfg.Environment["OPENAI_API_KEY"]; !exists {
		svccfg.Environment["OPENAI_API_KEY"] = &empty // disable auth; see https://github.com/DefangLabs/openai-access-gateway/pull/5
	}
	// svccfg.HealthCheck = &types.ServiceHealthCheckConfig{} TODO: add healthcheck
	svccfg.Image = "defangio/openai-access-gateway"
	if svccfg.Networks == nil {
		// New compose-go versions do not create networks for "provider:" services, so we need to create it here
		svccfg.Networks = make(map[string]*types.ServiceNetworkConfig)
	} else {
		delete(svccfg.Networks, "default") // remove the default network
	}
	svccfg.Networks[modelProviderNetwork] = nil
	svccfg.Ports = []types.ServicePortConfig{{Target: 80, Mode: Mode_HOST, Protocol: Protocol_TCP}}
	svccfg.Provider = nil // remove "provider:" because current backend will not accept it
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
			if _, ok := dependency.Environment[modelEnvKey]; !ok && len(modelVals) == 1 {
				dependency.Environment[modelEnvKey] = &modelVals[0]
			}
		}
	}
}

func getImageRepo(imageRepo string) string {
	image := strings.ToLower(imageRepo)
	image, _, _ = strings.Cut(image, ":")
	return image
}

func fixupPort(port composeTypes.ServicePortConfig) composeTypes.ServicePortConfig {
	switch port.Mode {
	case "":
		term.Warnf("port %d: no 'mode' was specified; defaulting to 'ingress' (add 'mode: ingress' to silence)", port.Target)
		fallthrough
	case Mode_INGRESS:
		// This code is unnecessarily complex because compose-go silently converts short `ports:` syntax to ingress+tcp
		if port.Protocol != Protocol_UDP {
			if port.Published != "" {
				term.Debugf("port %d: ignoring 'published: %s' in 'ingress' mode", port.Target, port.Published)
			}
			if port.AppProtocol == "" {
				// TCP ingress is not supported; assuming HTTP (add 'app_protocol: http' to silence)"
				port.AppProtocol = "http"
			}
		} else {
			term.Warnf("port %d: UDP ports default to 'host' mode (add 'mode: host' to silence)", port.Target)
			port.Mode = Mode_HOST
		}
	case Mode_HOST:
		// no-op
	default:
		panic(fmt.Sprintf("port %d: 'mode' should have been validated to be one of [host ingress] but got: %v", port.Target, port.Mode))
	}
	return port
}
