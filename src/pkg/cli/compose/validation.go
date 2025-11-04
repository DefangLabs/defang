package compose

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

type ListConfigNamesFunc func(context.Context) ([]string, error)

type ErrMissingConfig []string

func (e ErrMissingConfig) Error() string {
	return fmt.Sprintf("missing configs %q (https://docs.defang.io/docs/concepts/configuration)", ([]string)(e))
}

var ErrDockerfileNotFound = errors.New("dockerfile not found")

func ValidateProject(project *composeTypes.Project, mode modes.Mode) error {
	if project == nil {
		return errors.New("no project found")
	}
	// Copy the services map into a slice so we can sort them and have consistent output
	var services []composeTypes.ServiceConfig
	for _, svccfg := range project.Services {
		services = append(services, svccfg)
	}
	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})

	var errs []error
	for _, svccfg := range services {
		errs = append(errs, validateService(&svccfg, project, mode))
	}
	for i, svccfg := range services {
		for j := i + 1; j < len(services); j++ {
			if gcp.SafeLabelValue(svccfg.Name) == gcp.SafeLabelValue(services[j].Name) { // TODO: Shouldn't be just gcp specific
				errs = append(errs, fmt.Errorf("the service names %q and %q normalize to the same value, which causes a conflict. Please use distinct names that differ after normalization", svccfg.Name, services[j].Name))
			}
		}
	}
	return errors.Join(errs...)
}

func validateService(svccfg *composeTypes.ServiceConfig, project *composeTypes.Project, mode modes.Mode) error {
	if svccfg.ReadOnly {
		term.Debugf("service %q: unsupported compose directive: read_only", svccfg.Name)
	}
	if svccfg.Restart == "" {
		// This was a warning, but we don't really care and want to reduce the noise
		term.Debugf("service %q: missing compose directive: restart; assuming 'unless-stopped' (add 'restart' to silence)", svccfg.Name)
	} else if svccfg.Restart != "always" && svccfg.Restart != "unless-stopped" {
		term.Debugf("service %q: unsupported compose directive: restart; assuming 'unless-stopped' (add 'restart' to silence)", svccfg.Name)
	}
	if svccfg.ContainerName != "" {
		term.Debugf("service %q: unsupported compose directive: container_name", svccfg.Name)
	}
	if svccfg.Hostname != "" {
		return fmt.Errorf("service %q: unsupported compose directive: hostname; consider using 'domainname' instead", svccfg.Name)
	}
	if len(svccfg.DNSSearch) != 0 {
		return fmt.Errorf("service %q: unsupported compose directive: dns_search", svccfg.Name)
	}
	if len(svccfg.DNSOpts) != 0 {
		term.Debugf("service %q: unsupported compose directive: dns_opt", svccfg.Name)
	}
	if len(svccfg.DNS) != 0 {
		return fmt.Errorf("service %q: unsupported compose directive: dns", svccfg.Name)
	}
	if len(svccfg.Devices) != 0 {
		return fmt.Errorf("service %q: unsupported compose directive: devices", svccfg.Name)
	}
	if len(svccfg.DependsOn) != 0 {
		term.Debugf("service %q: unsupported compose directive: depends_on", svccfg.Name)
	}
	if len(svccfg.DeviceCgroupRules) != 0 {
		return fmt.Errorf("service %q: unsupported compose directive: device_cgroup_rules", svccfg.Name)
	}
	if len(svccfg.Entrypoint) > 0 {
		return fmt.Errorf("service %q: unsupported compose directive: entrypoint", svccfg.Name)
	}
	if len(svccfg.GroupAdd) > 0 {
		return fmt.Errorf("service %q: unsupported compose directive: group_add", svccfg.Name)
	}
	if len(svccfg.Ipc) > 0 {
		term.Debugf("service %q: unsupported compose directive: ipc", svccfg.Name)
	}
	if len(svccfg.Uts) > 0 {
		term.Debugf("service %q: unsupported compose directive: uts", svccfg.Name)
	}
	if svccfg.Isolation != "" {
		term.Debugf("service %q: unsupported compose directive: isolation", svccfg.Name)
	}
	if svccfg.MacAddress != "" {
		term.Debugf("service %q: unsupported compose directive: mac_address", svccfg.Name)
	}
	if len(svccfg.Labels) > 0 {
		term.Debugf("service %q: unsupported compose directive: labels", svccfg.Name) // TODO: add support for labels
	}
	if len(svccfg.Links) > 0 {
		term.Debugf("service %q: unsupported compose directive: links", svccfg.Name)
	}
	if svccfg.Logging != nil {
		term.Debugf("service %q: unsupported compose directive: logging", svccfg.Name)
	}
	for name := range svccfg.Networks {
		if _, ok := project.Networks[name]; !ok {
			// This was a warning, but we don't really care and want to reduce the noise
			term.Debugf("service %q: network %q is not defined in the top-level networks section", svccfg.Name, name)
		}
	}
	if len(svccfg.Volumes) > 0 {
		term.Warnf("service %q: unsupported compose directive: volumes", svccfg.Name) // TODO: add support for volumes
	}
	if len(svccfg.VolumesFrom) > 0 {
		term.Warnf("service %q: unsupported compose directive: volumes_from", svccfg.Name) // TODO: add support for volumes_from
	}
	if svccfg.Build != nil {
		_, err := filepath.Abs(svccfg.Build.Context)
		if err != nil {
			return fmt.Errorf("service %q: invalid build context: %w", svccfg.Name, err)
		}
		if svccfg.Build.Dockerfile != "" {
			if filepath.IsAbs(svccfg.Build.Dockerfile) {
				return fmt.Errorf("service %q: dockerfile path must be relative to the build context: %q", svccfg.Name, svccfg.Build.Dockerfile)
			}
			if strings.HasPrefix(svccfg.Build.Dockerfile, "../") {
				return fmt.Errorf("service %q: dockerfile path must be inside the build context: %q", svccfg.Name, svccfg.Build.Dockerfile)
			}
		}
		if svccfg.Build.SSH != nil {
			return fmt.Errorf("service %q: unsupported compose directive: build ssh", svccfg.Name)
		}
		if len(svccfg.Build.Labels) != 0 {
			term.Debugf("service %q: unsupported compose directive: build labels", svccfg.Name) // TODO: add support for Kaniko --label
		}
		if len(svccfg.Build.CacheFrom) != 0 {
			term.Debugf("service %q: unsupported compose directive: build cache_from", svccfg.Name)
		}
		if len(svccfg.Build.CacheTo) != 0 {
			term.Debugf("service %q: unsupported compose directive: build cache_to", svccfg.Name)
		}
		if svccfg.Build.NoCache {
			term.Debugf("service %q: unsupported compose directive: build no_cache", svccfg.Name)
		}
		if len(svccfg.Build.ExtraHosts) != 0 {
			return fmt.Errorf("service %q: unsupported compose directive: build extra_hosts", svccfg.Name)
		}
		if svccfg.Build.Isolation != "" {
			term.Debugf("service %q: unsupported compose directive: build isolation", svccfg.Name)
		}
		if svccfg.Build.Network != "" {
			return fmt.Errorf("service %q: unsupported compose directive: build network", svccfg.Name)
		}
		if len(svccfg.Build.Secrets) != 0 {
			return fmt.Errorf("service %q: unsupported compose directive: build secrets", svccfg.Name) // TODO: support build secrets
		}
		if len(svccfg.Build.Tags) != 0 {
			return fmt.Errorf("service %q: unsupported compose directive: build tags", svccfg.Name)
		}
		if len(svccfg.Build.Platforms) != 0 {
			return fmt.Errorf("service %q: unsupported compose directive: build platforms", svccfg.Name)
		}
		if svccfg.Build.Privileged {
			return fmt.Errorf("service %q: unsupported compose directive: build privileged", svccfg.Name)
		}
		if svccfg.Build.DockerfileInline != "" {
			return fmt.Errorf("service %q: unsupported compose directive: build dockerfile_inline", svccfg.Name)
		}
		if svccfg.Build.AdditionalContexts != nil {
			return fmt.Errorf("service %q: unsupported compose directive: build additional_contexts", svccfg.Name)
		}
		if svccfg.Build.Ulimits != nil {
			term.Warnf("service %q: unsupported compose directive: build ulimits", svccfg.Name) // TODO: add support for build ulimits
		}
	}
	for _, secret := range svccfg.Secrets {
		if !pkg.IsValidSecretName(secret.Source) {
			return fmt.Errorf("service %q: secret name is invalid: %q", svccfg.Name, secret.Source)
		}
		// secret.Target will always be automatically constructed by compose-go to "/run/secrets/<source>"
		if s, ok := project.Secrets[secret.Source]; !ok {
			// This was a warning, but we don't really care and want to reduce the noise
			term.Debugf("secret %q is not defined in the top-level secrets section", secret.Source)
		} else if s.Name != "" && s.Name != secret.Source {
			return fmt.Errorf("unsupported secret %q: cannot override name %q", secret.Source, s.Name) // TODO: support custom secret names
		} else if !s.External {
			term.Warnf("unsupported secret %q: not marked external:true", secret.Source) // TODO: support secrets from environment/file
		}
	}

	// check for compose file environment variables that may be sensitive
	for key, value := range svccfg.Environment {
		if value != nil {
			// format input as a key-value pair string
			input := key + "=" + *value

			// call detectConfig to check for sensitive information
			ds, err := detectConfig(input)
			if err != nil {
				return fmt.Errorf("service %q: %w", svccfg.Name, err)
			}

			// show warning if sensitive information is detected
			if len(ds) > 0 {
				term.Warnf("service %q: environment %q may contain sensitive information; consider using 'defang config set %s' to securely store this value", svccfg.Name, key, key)
				term.Debugf("service %q: environment %q may contain detected secrets of type: %q", svccfg.Name, key, ds)
			}
		}
	}

	err := validatePorts(svccfg.Ports)
	if err != nil {
		return fmt.Errorf("service %q: %w", svccfg.Name, err)
	}
	if svccfg.HealthCheck == nil || svccfg.HealthCheck.Disable {
		// Show a warning when we have ingress ports but no explicit healthcheck
		for _, port := range svccfg.Ports {
			if port.Mode == Mode_INGRESS {
				term.Warnf("service %q: ingress port %d without healthcheck; defaults to GET / HTTP/1.1", svccfg.Name, port.Target)
				break
			}
		}
	} else {
		timeout := 5.0
		if svccfg.HealthCheck.Timeout != nil {
			timeout = time.Duration(*svccfg.HealthCheck.Timeout).Seconds()
			if _, frac := math.Modf(timeout); frac != 0 {
				term.Warnf("service %q: healthcheck timeout must be a multiple of 1s", svccfg.Name)
			}
		}
		interval := 30.0 // default per compose spec
		if svccfg.HealthCheck.Interval != nil {
			interval = time.Duration(*svccfg.HealthCheck.Interval).Seconds()
			if _, frac := math.Modf(interval); frac != 0 {
				term.Warnf("service %q: healthcheck interval must be a multiple of 1s", svccfg.Name)
			}
		}
		// Technically this should test for <= but both interval and timeout have 30s as the default value
		if interval < timeout || timeout <= 0 {
			return fmt.Errorf("service %q: healthcheck timeout %fs must be positive and smaller than the interval %fs", svccfg.Name, timeout, interval)
		}
		if svccfg.HealthCheck.StartPeriod != nil {
			term.Debugf("service %q: unsupported compose directive: healthcheck start_period", svccfg.Name)
		}
		if svccfg.HealthCheck.StartInterval != nil {
			term.Debugf("service %q: unsupported compose directive: healthcheck start_interval", svccfg.Name)
		}
	}
	var replicas int
	var reservations *composeTypes.Resource
	if svccfg.Deploy != nil {
		if svccfg.Deploy.Mode != "" && svccfg.Deploy.Mode != "replicated" {
			return fmt.Errorf("service %q: unsupported compose directive: deploy mode: %q", svccfg.Name, svccfg.Deploy.Mode)
		}
		if svccfg.Deploy.UpdateConfig != nil {
			return fmt.Errorf("service %q: unsupported compose directive: deploy update_config", svccfg.Name)
		}
		if svccfg.Deploy.RollbackConfig != nil {
			return fmt.Errorf("service %q: unsupported compose directive: deploy rollback_config", svccfg.Name)
		}
		if svccfg.Deploy.RestartPolicy != nil {
			return fmt.Errorf("service %q: unsupported compose directive: deploy restart_policy", svccfg.Name)
		}
		if svccfg.Deploy.EndpointMode != "" {
			return fmt.Errorf("service %q: unsupported compose directive: deploy endpoint_mode", svccfg.Name)
		}
		if svccfg.Deploy.Resources.Limits != nil && svccfg.Deploy.Resources.Reservations == nil {
			term.Debugf("service %q: no reservations specified; using limits as reservations", svccfg.Name)
		}
		reservations = getResourceReservations(svccfg.Deploy.Resources)
		if reservations != nil && reservations.NanoCPUs < 0 { // "0" just means "as small as possible"
			return fmt.Errorf("service %q: invalid value for cpus: %v", svccfg.Name, reservations.NanoCPUs)
		}
		if len(svccfg.Deploy.Labels) > 0 {
			term.Debugf("service %q: unsupported compose directive: deploy labels", svccfg.Name)
		}
		if len(svccfg.Deploy.Placement.Constraints) != 0 || len(svccfg.Deploy.Placement.Preferences) != 0 || svccfg.Deploy.Placement.MaxReplicas != 0 {
			term.Debugf("service %q: unsupported compose directive: deploy placement", svccfg.Name)
		}
		if svccfg.Deploy.Replicas != nil {
			replicas = *svccfg.Deploy.Replicas
		}
	}
	if mode == modes.ModeHighAvailability && replicas < 2 && svccfg.Extensions["x-defang-autoscaling"] == nil {
		term.Warnf("service %q: high-availability mode requires at least 2 replicas or x-defang-autoscaling", svccfg.Name)
	}
	if reservations == nil || reservations.MemoryBytes == 0 {
		// Don't show this warning for managed pseudo-services like CDN
		if svccfg.Extensions["x-defang-static-files"] == nil {
			term.Warnf("service %q: missing memory reservation; using provider-specific defaults. Specify deploy.resources.reservations.memory to avoid out-of-memory errors", svccfg.Name)
		}
	}

	if dnsRoleVal := svccfg.Extensions["x-defang-dns-role"]; dnsRoleVal != nil {
		if _, ok := dnsRoleVal.(string); !ok {
			return fmt.Errorf("service %q: x-defang-dns-role must be a string", svccfg.Name)
		}
	}

	if staticFilesVal := svccfg.Extensions["x-defang-static-files"]; staticFilesVal != nil {
		_, str := staticFilesVal.(string)
		_, obj := staticFilesVal.(map[string]any)
		if !str && !obj {
			return fmt.Errorf(`service %q: x-defang-static-files must be a string or object {"folder": string, "redirects": string[]}`, svccfg.Name)
		}
	}

	repo := GetImageRepo(svccfg.Image)

	redisExtension, managedRedis := svccfg.Extensions["x-defang-redis"]
	if managedRedis {
		// Ensure the repo is a valid Redis repo
		if !IsRedisRepo(repo) {
			term.Warnf("service %q: managed Redis service should use a redis or valkey image", svccfg.Name)
		}
		if _, err = validateManagedStore(redisExtension); err != nil {
			return fmt.Errorf("service %q: %w", svccfg.Name, err)
		}
	}

	postgresExtension, managedPostgres := svccfg.Extensions["x-defang-postgres"]
	if managedPostgres {
		// Ensure the repo is a valid Postgres repo
		if !IsPostgresRepo(repo) {
			term.Warnf("service %q: managed Postgres service should use a postgres image", svccfg.Name)
		}
		if _, err = validateManagedStore(postgresExtension); err != nil {
			return fmt.Errorf("service %q: %w", svccfg.Name, err)
		}
	}

	mongodbExtension, managedMongodb := svccfg.Extensions["x-defang-mongodb"]
	if managedMongodb {
		// Ensure the repo is a valid MongoDB repo
		if !IsMongoRepo(repo) {
			term.Warnf("service %q: managed MongoDB service should use a mongo image", svccfg.Name)
		}
		if _, err = validateManagedStore(mongodbExtension); err != nil {
			return fmt.Errorf("service %q: %w", svccfg.Name, err)
		}
	}

	if !managedRedis && !managedPostgres && !managedMongodb && isStatefulImage(svccfg.Image) {
		term.Warnf("service %q: stateful service will lose data on restart; use a managed service instead", svccfg.Name)
	}

	for k := range svccfg.Extensions {
		switch k {
		case "x-defang-dns-role",
			"x-defang-static-files",
			"x-defang-redis",
			"x-defang-postgres",
			"x-defang-mongodb",
			"x-defang-llm",
			"x-defang-autoscaling":
			continue
		default:
			term.Warnf("service %q: unsupported compose extension: %q", svccfg.Name, k)
		}
	}

	return nil
}

func validatePorts(ports []composeTypes.ServicePortConfig) error {
	errs := make([]error, len(ports))
	for i, port := range ports {
		errs[i] = validatePort(port)
	}
	return errors.Join(errs...)
}

// We can changed to slices.contains when we upgrade to go 1.21 or above
var validProtocols = map[string]bool{"": true, "tcp": true, "udp": true, "http": true, "http2": true, "grpc": true}
var validModes = map[string]bool{"": true, "host": true, "ingress": true}

func validatePort(port composeTypes.ServicePortConfig) error {
	if port.Target < 1 || port.Target > 32767 {
		return fmt.Errorf("port %d: 'target' must be an integer between 1 and 32767", port.Target)
	}
	if port.HostIP != "" {
		return fmt.Errorf("port %d: 'host_ip' is not supported", port.Target)
	}
	if !validProtocols[port.Protocol] {
		return fmt.Errorf("port %d: 'protocol' not one of [tcp udp http http2 grpc]: %v", port.Target, port.Protocol)
	}
	if !validModes[port.Mode] {
		return fmt.Errorf("port %d: 'mode' not one of [host ingress]: %v", port.Target, port.Mode)
	}
	if port.Published != "" {
		portRange := strings.SplitN(port.Published, "-", 2)
		start, err := strconv.ParseUint(portRange[0], 10, 16)
		if err != nil {
			term.Warnf("port %d: 'published' range start should be an integer; ignoring 'published: %v'", port.Target, portRange[0])
		} else if len(portRange) == 2 {
			end, err := strconv.ParseUint(portRange[1], 10, 16)
			if err != nil {
				term.Warnf("port %d: 'published' range end should be an integer; ignoring 'published: %v'", port.Target, portRange[1])
			} else if start > end {
				term.Warnf("port %d: 'published' range start should be less than end; ignoring 'published: %v'", port.Target, port.Published)
			} else if port.Target < uint32(start) || port.Target > uint32(end) {
				term.Warnf("port %d: 'published' range should include 'target'; ignoring 'published: %v'", port.Target, port.Published)
			}
		} else {
			if start != uint64(port.Target) {
				term.Warnf("port %d: 'published' should be equal to 'target'; ignoring 'published: %v'", port.Target, port.Published)
			}
		}
	}

	return nil
}

func getResourceReservations(r composeTypes.Resources) *composeTypes.Resource {
	if r.Reservations == nil {
		// TODO: we might not want to default to all the limits, maybe only memory?
		return r.Limits
	}
	return r.Reservations
}

// Copied from shared/utils.ts but slightly modified to remove the negative-lookahead assertion
var interpolationRegex = regexp.MustCompile(`(?i)\$(\$)|\$(?:{([^}]+)}|([_a-z][_a-z0-9]*))|([^$]+)`) // [1] escaped dollar, [2] curly braces, [3] variable name, [4] literal

func ValidateProjectConfig(ctx context.Context, composeProject *composeTypes.Project, listConfigNamesFunc ListConfigNamesFunc) error {
	var names []string
	// make list of secrets
	for _, service := range composeProject.Services {
		for key, value := range service.Environment {
			if value == nil {
				names = append(names, key)
				continue
			}
			// check for variables used during interpolation
			for _, match := range interpolationRegex.FindAllStringSubmatch(*value, -1) {
				if match[2] != "" {
					names = append(names, match[2])
				}
				if match[3] != "" {
					names = append(names, match[3])
				}
			}
		}
	}

	if len(names) == 0 {
		return nil // no secrets to check
	}

	configs, err := listConfigNamesFunc(ctx)
	if err != nil {
		return err
	}

	// Deduplicate (sort + uniq)
	slices.Sort(names)
	names = slices.Compact(names)

	errMissingConfig := ErrMissingConfig{}
	for _, name := range names {
		if !slices.Contains(configs, name) {
			errMissingConfig = append(errMissingConfig, name)
		}
	}

	if len(errMissingConfig) > 0 {
		return errMissingConfig
	}

	return nil
}

func validateManagedStore(managedStore any) (bool, error) {
	switch managedStore := managedStore.(type) {
	case nil:
		return false, nil
	case bool:
		return managedStore, nil
	case string:
		return strconv.ParseBool(managedStore)
	case map[string]any:
		if downtime, ok := managedStore["allow-downtime"]; ok {
			if _, ok := downtime.(bool); !ok {
				return false, errors.New("'allow-downtime' must be a boolean")
			}
		}
		return true, nil
	default:
		return false, errors.New("expected parameters in managed storage definition field")
	}
}
