package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

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

type byocAws struct {
	driver        *cfn.AwsEcs
	setupDone     bool
	StackID       string // aka tenant
	privateDomain string
	customDomain  string
	cdTaskArn     awsecs.TaskArn
	privateLbIps  []string // TODO: grab these from the AWS API or outputs
	publicNatIps  []string // TODO: grab these from the AWS API or outputs
	albDnsName    string
}

var _ Client = (*byocAws)(nil)

func NewByocAWS(stackId, domain string) *byocAws {
	user := os.Getenv("USER") // TODO: sanitize
	if stackId == "" {
		stackId = user
	}
	return &byocAws{
		driver:        cfn.New(cdPrefix+user, aws.Region(pkg.Getenv("AWS_REGION", "us-west-2"))), // TODO: figure out how to get region
		StackID:       stackId,
		privateDomain: stackId + "." + projectName + ".internal", // must match the logic in ecs/common.ts
		customDomain:  domain,
		albDnsName:    "ecs-dev-alb-672419834.us-west-2.elb.amazonaws.com", // TODO: grab these from the AWS API or outputs
		// privateLbIps: nil,                                                 // TODO: grab these from the AWS API or outputs
		// publicNatIps: nil,                                                 // TODO: grab these from the AWS API or outputs
	}
}

func (b *byocAws) setUp(ctx context.Context) error {
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

func (b *byocAws) Deploy(ctx context.Context, req *v1.DeployRequest) (*v1.DeployResponse, error) {
	etag := pkg.RandomID()
	serviceInfos := []*v1.ServiceInfo{}
	for _, service := range req.Services {
		serviceInfo, err := b.update(ctx, service)
		if err != nil {
			return nil, err
		}
		serviceInfo.Etag = etag // same etag for all services
		serviceInfos = append(serviceInfos, serviceInfo)
	}

	// Ensure all service endpoints are unique
	endpoints := make(map[string]bool)
	for _, serviceInfo := range serviceInfos {
		for _, endpoint := range serviceInfo.Endpoints {
			if endpoints[endpoint] {
				return nil, fmt.Errorf("duplicate endpoint: %s", endpoint) // CodeInvalidArgument
			}
			endpoints[endpoint] = true
		}
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

func (byocAws) GetStatus(context.Context) (*v1.Status, error) {
	panic("not implemented: GetStatus")
}

func (byocAws) GetVersion(context.Context) (*v1.Version, error) {
	return &v1.Version{Fabric: cdVersion}, nil
}

func (byocAws) Token(context.Context, *v1.TokenRequest) (*v1.TokenResponse, error) {
	panic("not implemented: Token")
}

func (byocAws) RevokeToken(context.Context) error {
	panic("not implemented: RevokeToken")
}

func (b byocAws) Get(ctx context.Context, s *v1.ServiceID) (*v1.ServiceInfo, error) {
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

func (b *byocAws) runTask(ctx context.Context, cmd ...string) error {
	env := map[string]string{
		// "AWS_REGION":               b.driver.Region.String(), TODO: this should be the destination region, not the CD region
		"DOMAIN":                   b.customDomain,
		"PULUMI_BACKEND_URL":       fmt.Sprintf(`s3://%s?region=%s&awssdk=v2`, b.driver.BucketName, b.driver.Region),
		"PULUMI_CONFIG_PASSPHRASE": "asdf", // TODO: make customizable
		"STACK":                    b.StackID,
	}
	taskArn, err := b.driver.Run(ctx, env, cmd...)
	if err != nil {
		return err
	}
	b.cdTaskArn = taskArn
	return nil
}

func (b *byocAws) Delete(ctx context.Context, req *v1.DeleteRequest) (*v1.DeleteResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}
	if err := b.runTask(ctx, "npm", "start", "up", "TODO"); err != nil { // TODO: remove TODO payload (although it works fine!)
		return nil, err
	}
	return &v1.DeleteResponse{Etag: *b.cdTaskArn}, nil
}

func (byocAws) Publish(context.Context, *v1.PublishRequest) error {
	panic("not implemented: Publish")
}

func (b byocAws) getClusterNames() []string {
	return []string{
		projectName + "-" + b.StackID + "-cluster",
		projectName + "-" + b.StackID + "-gpu-cluster",
	}
}

func (b byocAws) GetServices(ctx context.Context) (*v1.ListServicesResponse, error) {
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

func (byocAws) GenerateFiles(context.Context, *v1.GenerateFilesRequest) (*v1.GenerateFilesResponse, error) {
	panic("not implemented: GenerateFiles")
}

func (b byocAws) PutSecret(ctx context.Context, secret *v1.SecretValue) error {
	return b.driver.PutSecret(ctx, secret.Name, b.StackID+"."+secret.Value)
}

func (b byocAws) ListSecrets(ctx context.Context) (*v1.Secrets, error) {
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

func (b *byocAws) CreateUploadURL(ctx context.Context, req *v1.UploadURLRequest) (*v1.UploadURLResponse, error) {
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

// byocServerStream is a wrapper around awsecs.EventStream that implements connect-like ServerStream
type byocServerStream struct {
	stream   awsecs.EventStream
	ctx      context.Context
	err      error
	response *v1.TailResponse
	etag     string
}

var _ ServerStream[v1.TailResponse] = (*byocServerStream)(nil)

func (bs *byocServerStream) Close() error {
	return nil
}

func (bs *byocServerStream) Err() error {
	return bs.err
}

func (bs *byocServerStream) Msg() *v1.TailResponse {
	return bs.response
}

type hasErrCh interface {
	Err() <-chan error
}

func (bs *byocServerStream) Receive() bool {
	var errCh <-chan error
	if errch, ok := bs.stream.(hasErrCh); ok {
		errCh = errch.Err()
	}
	select {
	case <-bs.ctx.Done(): // blocking
		bs.err = bs.ctx.Err()
		return false
	case err := <-errCh:
		bs.err = err
		return false
	case e := <-bs.stream.Events(): // blocking
		events, err := awsecs.ConvertEvents(e)
		if err != nil {
			bs.err = err
			return false
		}
		response := &v1.TailResponse{
			Entries: make([]*v1.LogEntry, len(events)),
		}
		for i, event := range events {
			parts := strings.Split(*event.LogGroupIdentifier, ":")
			if len(parts) > 1 && strings.HasPrefix(parts[1], cdPrefix) {
				parts := strings.Split(*event.LogStreamName, "/")
				response.Etag = parts[2] // taskID TODO: etag grab from tag?
				response.Service = "cd"
				response.Host = "pulumi"
			} else {
				parts := strings.Split(*event.LogStreamName, "/")
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
				Message:   *event.Message,
				Timestamp: timestamppb.New(time.UnixMilli(*event.Timestamp)),
				// Stderr:    false, TODO: detect somehow from source
			}
		}
		bs.response = response
		return true
	}
}

func (b *byocAws) Tail(ctx context.Context, req *v1.TailRequest) (ServerStream[v1.TailResponse], error) {
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
	var eventStream awsecs.EventStream
	if strings.HasPrefix(taskID, "arn:aws:ecs:") {
		eventStream, err = b.driver.TailTask(ctx, &taskID)
	} else {
		accountID := b.driver.GetAccountID()
		logGroupName := projectName + "-" + b.StackID + "-kaniko" // TODO: must match index.ts
		logGroupID := fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s", b.driver.Region.String(), accountID, logGroupName)
		eventStream, err = b.driver.TailLogGroups(ctx, b.driver.LogGroupARN, logGroupID)
	}
	return &byocServerStream{
		stream: eventStream,
		ctx:    ctx,
		etag:   req.Etag,
	}, err
}

// This functions was copied from Fabric controller and slightly modified to work with BYOC
func (b byocAws) update(ctx context.Context, service *v1.Service) (*v1.ServiceInfo, error) {
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
		si.LbIps = b.privateLbIps // only set LB IPs if there are ingress ports
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
			// return nil, fmt.Errorf("CNAME %q does not point to %q", service.Domainname, si.PublicFqdn)) FIXME: circular dependency // CodeFailedPrecondition
		}
	}
	si.NatIps = b.publicNatIps // TODO: even internal services use NAT now
	si.Status = "UPDATE_QUEUED"
	if si.Service.Build != nil {
		si.Status = "BUILD_QUEUED" // in SaaS, this gets overwritten by the ECS events for "kaniko"
	}
	return si, nil
}

func newQualifiedName(tenant string, name string) qualifiedName {
	return qualifiedName(fmt.Sprintf("%s.%s", tenant, name))
}

func (b byocAws) checkForMissingSecrets(ctx context.Context, secrets []*v1.Secret, tenantId string) *v1.Secret {
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

func (b byocAws) getEndpoint(fqn qualifiedName, port *v1.Port) string {
	safeFqn := dnsSafe(fqn)
	if port.Mode == v1.Mode_HOST {
		return fmt.Sprintf("%s.%s:%d", safeFqn, b.privateDomain, port.Target)
	} else {
		if b.customDomain == "" {
			return b.albDnsName
		}
		return fmt.Sprintf("%s--%d.%s", safeFqn, port.Target, b.customDomain)
	}
}

func (b byocAws) getFqdn(fqn qualifiedName, public bool) string {
	safeFqn := dnsSafe(fqn)
	if public {
		if b.customDomain == "" {
			return b.albDnsName
		}
		return fmt.Sprintf("%s.%s", safeFqn, b.customDomain)
	} else {
		return fmt.Sprintf("%s.%s", safeFqn, b.privateDomain)
	}
}

func dnsSafe(fqn qualifiedName) string {
	return strings.ReplaceAll(string(fqn), ".", "-")
}
