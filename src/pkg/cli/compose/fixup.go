package compose

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

const RAILPACK = "*Railpack"

type ServiceFixupper interface {
	FixupServices(ctx context.Context, project *composeTypes.Project) error
}

func FixupServices(ctx context.Context, provider client.Provider, project *composeTypes.Project, upload UploadMode) error {
	if fixupper, ok := provider.(ServiceFixupper); ok {
		if err := fixupper.FixupServices(ctx, project); err != nil {
			return err
		}
	}

	// Preload the current config so we can detect which environment variables should be passed as "secrets"
	config, err := provider.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: project.Name})
	if err != nil {
		slog.Debug("failed to load config", "err", err)
		config = &defangv1.Secrets{}
	}
	slices.Sort(config.Names) // sort for binary search

	accountInfo, err := provider.AccountInfo(ctx)
	if err != nil {
		slog.Debug("failed to get account info to fixup services", "err", err)
		accountInfo = &client.AccountInfo{}
	}

	// Fixup any pseudo services (this might create port configs, which will affect service name replacement by ReplaceServiceNameWithDNS)
	for _, svccfg := range project.Services {
		repo := GetImageRepo(svccfg.Image)

		_, managedRedis := svccfg.Extensions["x-defang-redis"]
		if managedRedis || IsRedisRepo(repo) {
			if err := fixupRedisService(&svccfg, provider, upload); err != nil {
				return fmt.Errorf("service %q: %w", svccfg.Name, err)
			}
		}

		_, managedPostgres := svccfg.Extensions["x-defang-postgres"]
		if managedPostgres || IsPostgresRepo(repo) {
			if err := fixupPostgresService(&svccfg, provider, upload); err != nil {
				return fmt.Errorf("service %q: %w", svccfg.Name, err)
			}
		}

		_, managedMongo := svccfg.Extensions["x-defang-mongodb"]
		if managedMongo || IsMongoRepo(repo) {
			if err := fixupMongoService(&svccfg, provider, upload); err != nil {
				return fmt.Errorf("service %q: %w", svccfg.Name, err)
			}
		}

		if svccfg.Provider != nil && svccfg.Provider.Type == "model" && svccfg.Image == "" && svccfg.Build == nil {
			fixupModelProvider(&svccfg, project, accountInfo)
		}

		if _, llm := svccfg.Extensions["x-defang-llm"]; llm {
			fixupLLM(&svccfg)
		}

		// Fixup ports, which affects service name replacement by ReplaceServiceNameWithDNS below
		for i, port := range svccfg.Ports {
			svccfg.Ports[i] = fixupPort(port)
		}

		// Ignore "build" config if we have "image", unless in --build or --force mode
		if svccfg.Image != "" && svccfg.Build != nil && upload != UploadModeDigest && upload != UploadModeForce {
			slog.WarnContext(ctx, fmt.Sprintf("service %q: using published image instead of rebuilding; pass --build to build and publish a new image", svccfg.Name))
			svccfg.Build = nil
		}

		// update the concrete service with the fixed up object
		project.Services[svccfg.Name] = svccfg
	}

	for name, model := range project.Models {
		model.Name = name // ensure the model has a name
		svccfg := fixupModel(model, project, accountInfo)
		project.Services[svccfg.Name] = *svccfg
	}

	svcNameReplacer := NewServiceNameReplacer(ctx, provider, project)

	for _, svccfg := range project.Services {
		// Upload the build context, if any; TODO: parallelize
		if svccfg.Build != nil {
			// Because of normalization, Dockerfile is always set to "Dockerfile" even if it was not specified in the compose file.
			if svccfg.Build.Dockerfile != "" {
				// Check if the dockerfile exists
				dockerfilePath := filepath.Join(svccfg.Build.Context, svccfg.Build.Dockerfile)
				if _, err := os.Stat(dockerfilePath); err != nil {
					slog.Debug("stat dockerfile", "path", dockerfilePath, "err", err)
					// In this case we know that the dockerfile is not in the location the compose file specifies,
					// so can assume that the dockerfile has been normalized to the default "Dockerfile".
					if svccfg.Build.Dockerfile != "Dockerfile" {
						// An explicit Dockerfile was specified, but it does not exist.
						return fmt.Errorf("service %q: %w: %q", svccfg.Name, ErrDockerfileNotFound, dockerfilePath)
					}
					// hint to CD that we want to use Railpack
					svccfg.Build.Dockerfile = RAILPACK

					// railpack generates images with `Entrypoint: "bash -c"`, and
					// compose-go normalizes string commands into arrays, for example:
					// `command: npm start` -> `command: [ "npm", "start" ]`. As a
					// result, the command which ultimately gets run is
					// `bash -c npm start`. When this gets run, `bash` will ignore
					// `start` and `npm` will get run in a subprocess--only printing
					// the help text. As it is common for users to type their service
					// command as a string, this cleanup step will help ensure the
					// command is run as intended by replacing `command: [ "npm", "start" ]`
					// with `command: [ "npm start" ]`.
					if len(svccfg.Command) > 1 {
						svccfg.Command = []string{pkg.ShellQuote(svccfg.Command...)}
					}
				}
			}

			if !strings.Contains(svccfg.Build.Context, "://") {
				// Pack the build context into a Archive and upload
				url, err := getRemoteBuildContext(ctx, provider, project.Name, svccfg.Name, svccfg.Build, upload)
				if err != nil {
					return err
				}
				svccfg.Build.Context = url
			}

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
				slog.WarnContext(ctx, fmt.Sprintf("service %q: skipping unset build argument %q", svccfg.Name, removedArgs))
			}
		}

		// Fixup secret references; secrets are supposed to be files, not env, but it's kept for backward compatibility
		for i, secret := range svccfg.Secrets {
			if i == 0 { // only warn once
				slog.WarnContext(ctx, fmt.Sprintf("service %q: secrets will be exposed as environment variables, not files (use 'environment' instead)", svccfg.Name))
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
					slog.WarnContext(ctx, fmt.Sprintf("service %q: skipping unset environment variable key", svccfg.Name))
					shownOnce = true
				}
				delete(svccfg.Environment, key) // remove the empty key; this is safe
				continue
			}
			if value == nil || *value == "${"+key+"}" {
				continue // actual value will be from config
			}

			// Check if the environment variable is an existing config; if so, mark it as such
			if _, ok := slices.BinarySearch(config.Names, key); ok {
				if svcNameReplacer.ContainsPrivateServiceName(*value) {
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
			slog.WarnContext(ctx, fmt.Sprintf("service %q: environment variable(s) %q will use the `defang config` value instead of adjusted service name", svccfg.Name, notAdjusted))
		}

		if len(overridden) > 0 {
			slog.WarnContext(ctx, fmt.Sprintf("service %q: environment variable(s) %q overridden by config", svccfg.Name, overridden))
		}

		_, scaling := svccfg.Extensions["x-defang-autoscaling"]
		if scaling {
			if _, ok := provider.(*client.PlaygroundProvider); ok {
				slog.WarnContext(ctx, fmt.Sprintf("service %q: auto-scaling is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name))
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

const liteLLMPort uint32 = 4000

func fixupLLM(svccfg *composeTypes.ServiceConfig) {
	// Strip tag/digest: only remove the suffix after ':' or '@' if it appears after the last '/'
	// so that a registry port (e.g. registry.example:5000/litellm:latest) is handled correctly.
	// Check '@' before ':' so that digest refs (name@sha256:hex) are cut at the '@', not inside the hex.
	sanitizedImage := svccfg.Image
	lastSlash := strings.LastIndex(sanitizedImage, "/")
	suffix := sanitizedImage[lastSlash+1:]
	if i := strings.IndexByte(suffix, '@'); i >= 0 {
		sanitizedImage = sanitizedImage[:lastSlash+1+i]
	} else if i := strings.IndexByte(suffix, ':'); i >= 0 {
		sanitizedImage = sanitizedImage[:lastSlash+1+i]
	}
	sanitizedImage = strings.ToLower(sanitizedImage)
	if strings.HasSuffix(sanitizedImage, "/litellm") && len(svccfg.Ports) == 0 {
		// HACK: we must have at least one host port to get a CNAME for the service
		// litellm listens on 4000 by default
		var port uint32 = liteLLMPort
		slog.Debug("adding LLM host port", "service", svccfg.Name, "port", port)
		svccfg.Ports = []composeTypes.ServicePortConfig{{Target: port, Mode: Mode_HOST, Protocol: Protocol_TCP}}
	}
}

func fixupPostgresService(svccfg *composeTypes.ServiceConfig, provider client.Provider, upload UploadMode) error {
	_, managedPostgres := svccfg.Extensions["x-defang-postgres"]
	if _, ok := provider.(*client.PlaygroundProvider); ok && managedPostgres && upload != UploadModeEstimate {
		slog.Warn(fmt.Sprintf("service %q: managed postgres is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name))
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
		slog.Debug("adding postgres host port", "service", svccfg.Name, "port", port)
		svccfg.Ports = []composeTypes.ServicePortConfig{{Target: port, Mode: Mode_HOST, Protocol: Protocol_TCP}}
	} else {
		fixupIngressPorts(svccfg)
	}
	return nil
}

func fixupMongoService(svccfg *composeTypes.ServiceConfig, provider client.Provider, upload UploadMode) error {
	_, managedMongo := svccfg.Extensions["x-defang-mongodb"]
	if _, ok := provider.(*client.PlaygroundProvider); ok && managedMongo && upload != UploadModeEstimate {
		slog.Warn(fmt.Sprintf("service %q: managed mongodb is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name))
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
		slog.Debug("adding mongodb host port", "service", svccfg.Name, "port", port)
		svccfg.Ports = []composeTypes.ServicePortConfig{{Target: port, Mode: Mode_HOST, Protocol: Protocol_TCP}}
	} else {
		fixupIngressPorts(svccfg)
	}
	return nil
}

func fixupRedisService(svccfg *composeTypes.ServiceConfig, provider client.Provider, upload UploadMode) error {
	_, managedRedis := svccfg.Extensions["x-defang-redis"]
	if _, ok := provider.(*client.PlaygroundProvider); ok && managedRedis && upload != UploadModeEstimate {
		slog.Warn(fmt.Sprintf("service %q: Managed redis is not supported in the Playground; consider using BYOC (https://s.defang.io/byoc)", svccfg.Name))
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
		slog.Debug("adding redis host port", "service", svccfg.Name, "port", port)
		svccfg.Ports = []composeTypes.ServicePortConfig{{Target: port, Mode: Mode_HOST, Protocol: Protocol_TCP}}
	} else {
		fixupIngressPorts(svccfg)
	}
	return nil
}

func fixupIngressPorts(svccfg *composeTypes.ServiceConfig) {
	for i, port := range svccfg.Ports {
		if port.Mode == Mode_INGRESS || port.Mode == "" {
			slog.Debug("changing port to host mode", "service", svccfg.Name, "port", port.Target)
			svccfg.Ports[i].Mode = Mode_HOST
		}
	}
}

// Declare a private network for the model provider
const modelProviderNetwork = "model_provider_private"

func fixupModel(model composeTypes.ModelConfig, project *composeTypes.Project, info *client.AccountInfo) *composeTypes.ServiceConfig {
	svccfg := &composeTypes.ServiceConfig{
		Name:       model.Name,
		Extensions: model.Extensions,
	}
	makeAccessGatewayService(svccfg, project, model.Model, info) // TODO: pass other model options too
	return svccfg
}

func fixupModelProvider(svccfg *composeTypes.ServiceConfig, project *composeTypes.Project, info *client.AccountInfo) {
	var model string
	if modelVals := svccfg.Provider.Options["model"]; len(modelVals) == 1 {
		model = modelVals[0]
	}
	makeAccessGatewayService(svccfg, project, model, info)
}

func makeAccessGatewayService(svccfg *composeTypes.ServiceConfig, project *composeTypes.Project, model string, info *client.AccountInfo) {
	// Local Docker sets [SERVICE]_URL and [SERVICE]_MODEL environment variables on the dependent services
	envName := strings.ToUpper(svccfg.Name) // TODO: handle characters that are not allowed in env vars, like '-'
	endpointEnvVar := envName + "_URL"
	urlVal := "http://" + svccfg.Name + ":" + strconv.FormatUint(uint64(liteLLMPort), 10) + "/v1/"
	modelEnvVar := envName + "_MODEL"

	resolvedModel, masterKey := configureAccessGateway(svccfg, project, model, info)
	wireDependentServices(project, svccfg.Name, urlVal, resolvedModel, masterKey, endpointEnvVar, modelEnvVar)
}

// configureAccessGateway resolves the model name for the target provider, configures
// the LiteLLM container (image, command, network, port), and derives LITELLM_MASTER_KEY.
// Returns the resolved model string and the master key pointer.
func configureAccessGateway(svccfg *composeTypes.ServiceConfig, project *composeTypes.Project, model string, info *client.AccountInfo) (string, *string) {
	// svccfg.Deploy.Resources.Reservations.Limits = &composeTypes.Resources{} TODO: avoid memory limits warning
	if svccfg.Environment == nil {
		svccfg.Environment = composeTypes.MappingWithEquals{}
	}

	alias := model
	switch info.Provider {
	case client.ProviderAWS:
		switch model {
		case "chat-default":
			model = "us.amazon.nova-2-lite-v1:0"
		case "embedding-default":
			model = "amazon.titan-embed-text-v2:0"
		}
		model = modelWithProvider(model, "bedrock")
		if info.Region != "" {
			svccfg.Environment["AWS_REGION"] = &info.Region
		}
	case client.ProviderGCP:
		switch model {
		case "chat-default":
			model = "gemini-2.5-flash"
		case "embedding-default":
			model = "gemini-embedding-001"
		}
		model = modelWithProvider(model, "vertex_ai")
		if info.AccountID != "" {
			svccfg.Environment["VERTEXAI_PROJECT"] = &info.AccountID
		}
		if info.Region != "" {
			svccfg.Environment["VERTEXAI_LOCATION"] = &info.Region
		}
	}

	// svccfg.HealthCheck = &composeTypes.ServiceHealthCheckConfig{} TODO: add healthcheck
	svccfg.Image = "litellm/litellm:v1.82.3-stable.patch.3"
	svccfg.Command = []string{"--drop_params", "--model", model, "--alias", alias}
	if svccfg.Networks == nil {
		// New compose-go versions do not create networks for "provider:" services, so we need to create it here
		svccfg.Networks = make(map[string]*composeTypes.ServiceNetworkConfig)
	} else {
		delete(svccfg.Networks, "default") // remove the default network
	}
	svccfg.Networks[modelProviderNetwork] = nil
	svccfg.Ports = []composeTypes.ServicePortConfig{{Target: liteLLMPort, Mode: Mode_HOST, Protocol: Protocol_TCP}}
	svccfg.Provider = nil // remove "provider:" because current backend will not accept it
	project.Networks[modelProviderNetwork] = composeTypes.NetworkConfig{Name: modelProviderNetwork}

	masterKey, exists := svccfg.Environment["LITELLM_MASTER_KEY"]
	if !exists {
		openAIKey := ""
		for _, service := range project.Services {
			if _, ok := service.DependsOn[svccfg.Name]; ok {
				if key, ok := service.Environment["OPENAI_API_KEY"]; ok {
					if openAIKey == "" {
						openAIKey = *key
					} else if *key != openAIKey {
						slog.Error(fmt.Sprintf("multiple different OPENAI_API_KEY values found in services depending on %q", svccfg.Name))
						break
					}
				}
			}
		}
		if openAIKey == "" {
			key := "networkisalreadyprivate"
			masterKey = &key
		} else {
			masterKey = &openAIKey
		}
		svccfg.Environment["LITELLM_MASTER_KEY"] = masterKey
	}

	return model, masterKey
}

// wireDependentServices injects URL, model, and API-key env vars and adds the
// model-provider network to every service that depends on svcName.
func wireDependentServices(project *composeTypes.Project, svcName, urlVal, model string, masterKey *string, endpointEnvVar, modelEnvVar string) {
	for name, dependency := range project.Services {
		changed := false

		if _, ok := dependency.DependsOn[svcName]; ok {
			if dependency.Environment == nil {
				dependency.Environment = make(composeTypes.MappingWithEquals)
			}
			dependency.Networks[modelProviderNetwork] = nil
			if _, ok := dependency.Environment[endpointEnvVar]; !ok {
				dependency.Environment[endpointEnvVar] = &urlVal
			}
			if _, ok := dependency.Environment[modelEnvVar]; !ok && model != "" {
				dependency.Environment[modelEnvVar] = &model
			}
			if _, ok := dependency.Environment["OPENAI_API_KEY"]; !ok {
				dependency.Environment["OPENAI_API_KEY"] = masterKey
			}
			changed = true
		}

		if modelDep, ok := dependency.Models[svcName]; ok {
			endpointVar := endpointEnvVar
			if modelDep != nil && modelDep.EndpointVariable != "" {
				endpointVar = modelDep.EndpointVariable
			}
			modelVar := modelEnvVar
			if modelDep != nil && modelDep.ModelVariable != "" {
				modelVar = modelDep.ModelVariable
			}
			if dependency.Environment == nil {
				dependency.Environment = make(composeTypes.MappingWithEquals)
			}
			dependency.Networks[modelProviderNetwork] = nil
			if _, ok := dependency.Environment[endpointVar]; !ok {
				dependency.Environment[endpointVar] = &urlVal
			}
			if _, ok := dependency.Environment[modelVar]; !ok && model != "" {
				dependency.Environment[modelVar] = &model
			}
			// If the model is not already declared as a dependency, add it
			if _, ok := dependency.DependsOn[svcName]; !ok {
				if dependency.DependsOn == nil {
					dependency.DependsOn = make(map[string]composeTypes.ServiceDependency)
				}
				dependency.DependsOn[svcName] = composeTypes.ServiceDependency{
					Condition: composeTypes.ServiceConditionStarted,
					Required:  true,
				}
			}
			changed = true
		}

		if changed {
			project.Services[name] = dependency
		}
	}
}

func modelWithProvider(model, prefix string) string {
	if strings.Contains(model, "/") {
		return model // already has a provider prefix
	}
	return prefix + "/" + model
}

func GetImageRepo(image string) string {
	repo, _, _ := strings.Cut(image, ":")
	return strings.ToLower(repo)
}

func fixupPort(port composeTypes.ServicePortConfig) composeTypes.ServicePortConfig {
	switch port.Mode {
	case "":
		slog.Warn(fmt.Sprintf("port %d: no 'mode' was specified; defaulting to 'ingress' (add 'mode: ingress' to silence)", port.Target))
		fallthrough
	case Mode_INGRESS:
		// This code is unnecessarily complex because compose-go silently converts short `ports:` syntax to ingress+tcp
		if port.Protocol == Protocol_UDP {
			slog.Warn(fmt.Sprintf("port %d: UDP ports default to 'host' mode (add 'mode: host' to silence)", port.Target))
			port.Mode = Mode_HOST
		} else {
			if port.Published != "" {
				slog.Debug("ignoring 'published' in 'ingress' mode", "port", port.Target, "published", port.Published)
			}
			if port.AppProtocol == "" {
				// TCP ingress is not supported; assuming HTTP (add 'app_protocol: http' to silence)"
				port.AppProtocol = "http"
			}
		}
	case Mode_HOST:
		// no-op
	default:
		panic(fmt.Sprintf("port %d: 'mode' should have been validated to be one of [host ingress] but got: %v", port.Target, port.Mode))
	}
	return port
}

func IsPostgresRepo(repo string) bool {
	// TODO: check if managed postgres supports postgis
	return strings.HasSuffix(repo, "postgres") || strings.HasSuffix(repo, "pgvector")
}

func IsRedisRepo(repo string) bool {
	return strings.HasSuffix(repo, "redis") || strings.HasSuffix(repo, "redis-stack") ||
		strings.HasSuffix(repo, "valkey") || strings.HasSuffix(repo, "valkey-bundle")
}

func IsMongoRepo(repo string) bool {
	return strings.HasSuffix(repo, "mongo")
}
