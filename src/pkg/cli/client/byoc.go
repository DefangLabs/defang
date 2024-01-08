package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/aws"
	awsecs "github.com/defang-io/defang/src/pkg/aws/ecs"
	"github.com/defang-io/defang/src/pkg/aws/ecs/cfn"
	"github.com/defang-io/defang/src/pkg/http"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	maxCpus       = 2
	maxGpus       = 1
	maxMemoryMiB  = 8192
	maxReplicas   = 2
	maxServices   = 6
	maxShmSizeMiB = 30720
	cdVersion     = "latest"
	projectName   = "bootstrap" // must match the projectName in index.ts
	cdPrefix      = "cd-"       // renaming this practically deletes the Pulumi state
)

var (
	privateLbIps []string = nil
	publicNatIps []string = nil
)

type byoc struct {
	driver         *cfn.AwsEcs
	setupDone      bool
	StackID        string // aka tenant
	privateDomain  string
	customerDomain string
	cdTaskArn      awsecs.TaskArn
}

var _ Client = (*byoc)(nil)

func NewByocClient(projectId string) *byoc {
	user := os.Getenv("USER") // TODO: sanitize
	if projectId == "" {
		projectId = user
	}
	return &byoc{
		driver:         cfn.New(cdPrefix+user, aws.Region(pkg.Getenv("AWS_REGION", "us-west-2"))), // TODO: figure out how to get region
		StackID:        projectId,
		privateDomain:  projectId + "." + projectName + ".internal",
		customerDomain: "gnafed.click", // TODO: make configurable/optional
	}
}

func (b *byoc) setUp(ctx context.Context) error {
	if b.setupDone {
		return nil
	}
	// TODO: can we stick to the vanilla pulumi-nodejs image?
	if err := b.driver.SetUp(ctx, "docker.io/defangio/cd:"+cdVersion, 512_000_000, "linux/amd64"); err != nil {
		return err
	}
	b.setupDone = true
	return nil
}

func (b *byoc) Deploy(ctx context.Context, req *v1.DeployRequest) (*v1.DeployResponse, error) {
	etag := pkg.RandomID()
	serviceInfos := []*v1.ServiceInfo{}
	for _, service := range req.Services {
		serviceInfo, err := b.update(ctx, service)
		if err != nil {
			return nil, err
		}
		serviceInfo.Etag = etag
		serviceInfos = append(serviceInfos, serviceInfo)
	}
	data, err := proto.Marshal(&v1.ListServicesResponse{
		Services: serviceInfos,
	})
	if err != nil {
		return nil, err
	}

	var payloadString string
	if len(data) < 1000 {
		// Small payloads can be sent as base64-encoded command-line argument
		payloadString = base64.StdEncoding.EncodeToString(data)
	} else {
		url, err := b.driver.CreateUploadURL(ctx, "")
		if err != nil {
			return nil, err
		}

		// Do an HTTP PUT to the generated URL
		resp, err := http.Put(ctx, url, "application/protobuf", bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("unexpected status code during upload: %d", resp.StatusCode)
		}
		payloadString = http.RemoveQueryParam(url)
	}

	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	return &v1.DeployResponse{
		Services: serviceInfos,
		Etag:     etag,
	}, b.runTask(ctx, "npm", "start", "up", payloadString)
}

func (byoc) GetStatus(context.Context) (*v1.Status, error) {
	panic("not implemented: GetStatus")
}

func (byoc) GetVersion(context.Context) (*v1.Version, error) {
	return &v1.Version{Fabric: cdVersion}, nil
}

func (byoc) Token(context.Context, *v1.TokenRequest) (*v1.TokenResponse, error) {
	panic("not implemented: Token")
}

func (byoc) RevokeToken(context.Context) error {
	panic("not implemented: RevokeToken")
}

func (b byoc) Get(ctx context.Context, s *v1.ServiceID) (*v1.ServiceInfo, error) {
	all, err := b.GetServices(ctx)
	if err != nil {
		return nil, err
	}
	for _, service := range all.Services {
		if service.Service.Name == s.Name {
			return service, nil
		}
	}
	return nil, errors.New("service not found") // CodeNotFound
}

func (b *byoc) runTask(ctx context.Context, cmd ...string) error {
	env := map[string]string{
		// "AWS_REGION":                 b.driver.Region.String(), this should be the destination region, not the CD region
		"DOMAIN":                     b.customerDomain, // TODO: make optional
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",          // suppresses "npm notice" update messages TODO: move to Dockerfile.cd
		"PULUMI_BACKEND_URL":         fmt.Sprintf(`s3://%s?region=%s&awssdk=v2`, b.driver.BucketName, b.driver.Region),
		"PULUMI_CONFIG_PASSPHRASE":   "asdf", // TODO: make customizable
		"STACK":                      b.StackID,
	}
	taskArn, err := b.driver.Run(ctx, env, cmd...)
	if err != nil {
		return err
	}
	b.cdTaskArn = taskArn
	return nil
}

func (b *byoc) Delete(ctx context.Context, req *v1.DeleteRequest) (*v1.DeleteResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}
	if err := b.runTask(ctx, "npm", "start", "up", "TODO"); err != nil {
		return nil, err
	}
	return &v1.DeleteResponse{Etag: *b.cdTaskArn}, nil
}

func (byoc) Publish(context.Context, *v1.PublishRequest) error {
	panic("not implemented: Publish")
}

func (b byoc) getClusterNames() []string {
	return []string{
		projectName + "-" + b.StackID + "-cluster",
		projectName + "-" + b.StackID + "-gpu-cluster",
	}
}

func (b byoc) GetServices(ctx context.Context) (*v1.ListServicesResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}
	var maxResults int32 = 100 // the maximum allowed by AWS
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	clusters := make(map[string][]string)
	ecsClient := ecs.NewFromConfig(cfg)
	for _, clusterName := range b.getClusterNames() {
		serviceArns, err := ecsClient.ListServices(ctx, &ecs.ListServicesInput{
			Cluster:    &clusterName,
			MaxResults: &maxResults, // TODO: handle pagination
		})
		if err != nil {
			// return nil, err
			// TODO: ignore ClusterNotFoundException
			continue
		}
		clusters[clusterName] = serviceArns.ServiceArns
	}
	/* TODO: use this once namespaces are working
	s, err := ecsClient.ListServicesByNamespace(ctx, &ecs.ListServicesByNamespaceInput{
		MaxResults: &maxResults,
		Namespace:  &b.privateDomain,
	})
	if err != nil {
		// Ignore NamespaceNotFoundException
		var namespaceNotFound *ecsTypes.NamespaceNotFoundException
		if errors.As(err, &namespaceNotFound) {
			println("ns not found")
			return &v1.ListServicesResponse{}, nil
		}
		return nil, err
	}
	if len(s.ServiceArns) == 0 {
		println("no services found")
		return &v1.ListServicesResponse{}, nil
	}
	// Convert service ARNs to cluster name(s)
	for _, arn := range s.ServiceArns {
		// arn:aws:ecs:us-west-2:532501343364:service/ecs-edw-cluster/fabric-31fdcb4
		resourcePath := strings.Split(arn, ":")[5]
		parts := strings.Split(resourcePath, "/")
		cluster, service := parts[1], parts[2]
		clusters[cluster] = append(clusters[cluster], service)
	}*/
	// Query services for each cluster
	serviceInfos := []*v1.ServiceInfo{}
	for cluster, serviceNames := range clusters {
		if len(serviceNames) == 0 {
			continue
		}
		dso, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Services: serviceNames,
			Cluster:  &cluster,
		})
		if err != nil {
			return nil, err
		}
		for _, service := range dso.Services {
			// TODO: get the service definition from the task definition or tags
			serviceInfos = append(serviceInfos, &v1.ServiceInfo{
				Service: &v1.Service{
					Name: *service.ServiceName,
				},
			})
		}
	}
	return &v1.ListServicesResponse{Services: serviceInfos}, nil
}

func (byoc) GenerateFiles(context.Context, *v1.GenerateFilesRequest) (*v1.GenerateFilesResponse, error) {
	panic("not implemented: GenerateFiles")
}

func (b byoc) PutSecret(ctx context.Context, secret *v1.SecretValue) error {
	return b.driver.PutSecret(ctx, secret.Name, b.StackID+"."+secret.Value)
}

func (b byoc) ListSecrets(ctx context.Context) (*v1.Secrets, error) {
	awsSecrets, err := b.driver.ListSecretsByPrefix(ctx, b.StackID)
	if err != nil {
		return nil, err
	}
	secrets := make([]string, len(awsSecrets))
	for i, secret := range awsSecrets {
		secrets[i] = strings.TrimPrefix(secret, b.StackID+".")
	}
	return &v1.Secrets{Names: secrets}, nil
}

func (b *byoc) CreateUploadURL(ctx context.Context, req *v1.UploadURLRequest) (*v1.UploadURLResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	url, err := b.driver.CreateUploadURL(ctx, req.Digest)
	if err != nil {
		return nil, err
	}
	return &v1.UploadURLResponse{
		Url: url,
	}, nil
}

type byocStreamer struct {
	*awsecs.LogStreamer
	ctx      context.Context
	err      error
	response *v1.TailResponse
}

func (bs *byocStreamer) Close() error {
	return nil
}

func (bs *byocStreamer) Err() error {
	return bs.err
}

func (bs *byocStreamer) Msg() *v1.TailResponse {
	return bs.response
}

func (bs *byocStreamer) Receive() bool {
	events, err := bs.LogStreamer.Receive(bs.ctx)
	response := &v1.TailResponse{
		Entries: make([]*v1.LogEntry, len(events)),
	}
	for i, event := range events {
		if strings.HasPrefix(event.LogGroupID, cdPrefix) {
			parts := strings.Split(event.LogStream, "/")
			response.Etag = parts[2] // taskID TODO: grab from tag?
			response.Service = "cd"
			response.Host = "pulumi"
		} else {
			parts := strings.Split(event.LogStream, "/")
			if len(parts) == 3 {
				response.Host = parts[2] // taskID
				parts = strings.SplitN(parts[1], "_", 2)
				if len(parts) == 2 {
					service, etag := parts[0], parts[1]
					response.Service = service
					response.Etag = etag
				}
			}
		}
		response.Entries[i] = &v1.LogEntry{
			Message:   event.Message,
			Timestamp: timestamppb.New(event.Timestamp),
			// Stderr:    false, TODO: detect
		}
	}
	bs.response = response
	var resourceNotFound *types.ResourceNotFoundException
	if errors.As(err, &resourceNotFound) || err == io.EOF {
		err = nil // TODO: return error if the task has stopped
		pkg.SleepWithContext(bs.ctx, time.Second/10)
	}
	bs.err = err
	return err == nil
}

func (b *byoc) Tail(ctx context.Context, req *v1.TailRequest) (ServerStream[v1.TailResponse], error) {
	if req.Service != "" && req.Service != "cd" {
		return nil, errors.New("service not found") // TODO: implement querying other services/tasks
	}
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}
	// TODO: support --since
	taskID := req.Etag
	if taskID == "" && req.Service == "cd" {
		taskID = *b.cdTaskArn
	}
	// How to tail multiple tasks/services at once?
	//  * No Etag, no service:	tail all tasks/services
	//  * Etag, no service: 	tail all tasks/services with that Etag
	//  * No Etag, service:		tail all tasks/services with that service name
	//  * Etag, service:		tail that task/service
	var err error
	var streamer *awsecs.LogStreamer
	if strings.HasPrefix(taskID, "arn:aws:ecs:") {
		streamer, err = b.driver.TailTask(ctx, &taskID)
	} else {
		accountID := "532501343364"                               // FIXME: hard-coded
		logGroupName := projectName + "-" + b.StackID + "-kaniko" // TODO: must match index.ts
		logGroupID := fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s", b.driver.Region.String(), accountID, logGroupName)
		streamer, err = b.driver.TailLogGroups(ctx, b.driver.LogGroupARN, logGroupID)
	}
	return &byocStreamer{
		LogStreamer: streamer,
		ctx:         ctx,
	}, err
}

func (b byoc) update(ctx context.Context, service *v1.Service) (*v1.ServiceInfo, error) {
	if service.Name == "" {
		return nil, errors.New("service name is required") // CodeInvalidArgument
	}
	if service.Build != nil {
		if service.Build.Context == "" {
			return nil, errors.New("build.context is required") // CodeInvalidArgument
		}
		if service.Build.ShmSize > maxShmSizeMiB || service.Build.ShmSize < 0 {
			return nil, fmt.Errorf("build.shm_size exceeds quota (max %d MiB)", maxShmSizeMiB) // CodeInvalidArgument
		}
		// TODO: consider stripping the pre-signed query params from the URL, but only if it's an S3 URL. (Not ideal because it would cause a diff in Pulumi.)
	} else {
		if service.Image == "" {
			return nil, errors.New("missing image") // CodeInvalidArgument
		}
	}

	uniquePorts := make(map[uint32]bool)
	for _, port := range service.Ports {
		if port.Target < 1 || port.Target > 32767 {
			return nil, fmt.Errorf("port %d is out of range", port.Target) // CodeInvalidArgument
		}
		if port.Mode == v1.Mode_INGRESS {
			if port.Protocol == v1.Protocol_TCP || port.Protocol == v1.Protocol_UDP {
				return nil, fmt.Errorf("mode:INGRESS is not supported by protocol:%s", port.Protocol) // CodeInvalidArgument
			}
		}
		if uniquePorts[port.Target] {
			return nil, fmt.Errorf("duplicate port %d", port.Target) // CodeInvalidArgument
		}
		uniquePorts[port.Target] = true
	}
	if service.Healthcheck != nil && len(service.Healthcheck.Test) > 0 {
		switch service.Healthcheck.Test[0] {
		case "CMD":
			if len(service.Healthcheck.Test) < 3 {
				return nil, errors.New("invalid CMD healthcheck; expected a command and URL") // CodeInvalidArgument
			}
			if !strings.HasSuffix(service.Healthcheck.Test[1], "curl") && !strings.HasSuffix(service.Healthcheck.Test[1], "wget") {
				return nil, errors.New("invalid CMD healthcheck; expected curl or wget") // CodeInvalidArgument
			}
			hasHttpUrl := false
			for _, arg := range service.Healthcheck.Test[2:] {
				if u, err := url.Parse(arg); err == nil && u.Scheme == "http" {
					hasHttpUrl = true
					break
				}
			}
			if !hasHttpUrl {
				return nil, errors.New("invalid CMD healthcheck; missing HTTP URL") // CodeInvalidArgument
			}
		case "HTTP": // TODO: deprecate
			if len(service.Healthcheck.Test) != 2 || !strings.HasPrefix(service.Healthcheck.Test[1], "/") {
				return nil, errors.New("invalid HTTP healthcheck; expected an absolute path") // CodeInvalidArgument
			}
		case "NONE": // OK
			if len(service.Healthcheck.Test) != 1 {
				return nil, errors.New("invalid NONE healthcheck; expected no arguments") // CodeInvalidArgument
			}
		default:
			return nil, fmt.Errorf("unsupported healthcheck: %v", service.Healthcheck.Test) // CodeInvalidArgument
		}
	}

	if service.Deploy != nil {
		// TODO: create proper per-tenant per-stage quotas for these
		if service.Deploy.Replicas > maxReplicas {
			return nil, fmt.Errorf("replicas exceeds quota (max %d)", maxReplicas) // CodeInvalidArgument
		}
		if service.Deploy.Resources != nil && service.Deploy.Resources.Reservations != nil {
			if service.Deploy.Resources.Reservations.Cpus > maxCpus || service.Deploy.Resources.Reservations.Cpus < 0 {
				return nil, fmt.Errorf("cpus exceeds quota (max %d vCPU)", maxCpus) // CodeInvalidArgument
			}
			if service.Deploy.Resources.Reservations.Memory > maxMemoryMiB || service.Deploy.Resources.Reservations.Memory < 0 {
				return nil, fmt.Errorf("memory exceeds quota (max %d MiB)", maxMemoryMiB) // CodeInvalidArgument
			}
			for _, device := range service.Deploy.Resources.Reservations.Devices {
				if len(device.Capabilities) != 1 || device.Capabilities[0] != "gpu" {
					return nil, errors.New("only GPU devices are supported") // CodeInvalidArgument
				}
				if device.Driver != "" && device.Driver != "nvidia" {
					return nil, errors.New("only nvidia GPU devices are supported") // CodeInvalidArgument
				}
				if device.Count > maxGpus {
					return nil, fmt.Errorf("gpu count exceeds quota (max %d)", maxGpus) // CodeInvalidArgument
				}
			}
		}
	}
	// Check to make sure all required secrets are present in the secrets store
	if missing := b.checkForMissingSecrets(ctx, service.Secrets, b.StackID); missing != nil {
		return nil, fmt.Errorf("missing secret %s", missing) // retryable CodeFailedPrecondition
	}
	si := &v1.ServiceInfo{
		Service: service,
		Tenant:  b.StackID,
		Etag:    pkg.RandomID(), // TODO: could be hash for dedup/idempotency
	}

	hasHost := false
	hasIngress := false
	fqn := newQualifiedName(b.StackID, service.Name)
	for _, port := range service.Ports {
		hasIngress = hasIngress || port.Mode == v1.Mode_INGRESS
		hasHost = hasHost || port.Mode == v1.Mode_HOST
		si.Endpoints = append(si.Endpoints, b.getEndpoint(fqn, port))
	}
	if hasIngress {
		si.LbIps = privateLbIps // only set LB IPs if there are ingress ports
		si.PublicFqdn = b.getFqdn(fqn, true)
	}
	if hasHost {
		si.PrivateFqdn = b.getFqdn(fqn, false)
	}
	if service.Domainname != "" {
		if !hasIngress {
			return nil, errors.New("domainname requires at least one ingress port") // retryable CodeFailedPrecondition
		}
		// Do a DNS lookup for Domainname and confirm it's indeed a CNAME to the service's public FQDN
		cname, err := net.LookupCNAME(service.Domainname)
		if err != nil {
			log.Printf("error looking up CNAME %q: %v\n", service.Domainname, err)
			// Do not expose the error to the client, but fail the request with FailedPrecondition
		}
		if strings.TrimSuffix(cname, ".") != si.PublicFqdn {
			log.Printf("CNAME %q does not point to %q\n", service.Domainname, si.PublicFqdn) // TODO: send to Loki tenant log
			// return nil, fmt.Errorf("CNAME %q does not point to %q", service.Domainname, si.PublicFqdn)) FIXME: circular dependenc // CodeFailedPrecondition
		}
	}
	si.NatIps = publicNatIps // TODO: even internal services use NAT now
	return si, nil
}

func newQualifiedName(tenant string, name string) qualifiedName {
	return qualifiedName(fmt.Sprintf("%s.%s", tenant, name))
}

func (b byoc) checkForMissingSecrets(ctx context.Context, secrets []*v1.Secret, tenantId string) *v1.Secret {
	if len(secrets) == 1 {
		// Avoid fetching the list of secrets from AWS by only checking the one we need
		fqn := newQualifiedName(tenantId, secrets[0].Source)
		found, err := b.driver.IsValidSecret(ctx, fqn)
		if err != nil {
			log.Printf("error checking secret: %v\n", err)
		}
		if !found {
			return secrets[0]
		}
	} else if len(secrets) > 1 {
		// Avoid multiple calls to AWS by sorting the list and then doing a binary search
		sorted, err := b.driver.ListSecretsByPrefix(ctx, b.StackID)
		if err != nil {
			log.Println("error listing secrets:", err)
		}
		for _, secret := range secrets {
			fqn := newQualifiedName(tenantId, secret.Source)
			if i := sort.Search(len(sorted), func(i int) bool {
				return sorted[i] >= fqn
			}); i >= len(sorted) || sorted[i] != fqn {
				return secret // secret not found
			}
		}
	}
	return nil // all secrets found (or none specified)
}

type qualifiedName = string // legacy

func (b byoc) getEndpoint(fqn qualifiedName, port *v1.Port) string {
	safeFqn := dnsSafe(fqn)
	if port.Mode == v1.Mode_HOST {
		return fmt.Sprintf("%s.%s:%d", safeFqn, b.privateDomain, port.Target)
	} else {
		return fmt.Sprintf("%s--%d.%s", safeFqn, port.Target, b.customerDomain)
	}
}

func (b byoc) getFqdn(fqn qualifiedName, public bool) string {
	safeFqn := dnsSafe(fqn)
	if public {
		return fmt.Sprintf("%s.%s", safeFqn, b.customerDomain)
	} else {
		return fmt.Sprintf("%s.%s", safeFqn, b.privateDomain)
	}
}

func dnsSafe(fqn qualifiedName) string {
	return strings.ReplaceAll(string(fqn), ".", "-")
}
