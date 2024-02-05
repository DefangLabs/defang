package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/aws"
	awsecs "github.com/defang-io/defang/src/pkg/aws/ecs"
	"github.com/defang-io/defang/src/pkg/aws/ecs/cfn"
	"github.com/defang-io/defang/src/pkg/http"
	"github.com/defang-io/defang/src/pkg/logs"
	"github.com/defang-io/defang/src/pkg/quota"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	cdVersion    = "v0.4.50-241-gb0e7e8cc" // will cause issues if two clients with different versions are connected to the same stack
	projectName  = "defang"                // TODO: support multiple projects
	cdTaskPrefix = "defang-cd"             // WARNING: renaming this practically deletes the Pulumi state
)

type byocAws struct {
	*GrpcClient

	cdTasks       map[string]awsecs.TaskArn
	customDomain  string
	driver        *cfn.AwsEcs
	privateDomain string
	privateLbIps  []string
	publicNatIps  []string
	pulumiStack   string
	quota         quota.Quotas
	setupDone     bool
	tenantID      string
}

var _ Client = (*byocAws)(nil)

func NewByocAWS(tenantId, domain string, defClient *GrpcClient) *byocAws {
	const stage = "defang" // must match prefix in secrets.go
	return &byocAws{
		cdTasks:       make(map[string]awsecs.TaskArn),
		customDomain:  domain,
		driver:        cfn.New(cdTaskPrefix, aws.Region("")), // default region
		GrpcClient:    defClient,
		privateDomain: projectName + "." + tenantId + ".internal", // must match the logic in ecs/common.ts
		pulumiStack:   tenantId + "-" + stage,
		quota: quota.Quotas{
			// These serve mostly to pevent fat-finger errors in the CLI or Compose files
			Cpus:       16,
			Gpus:       8,
			MemoryMiB:  65536,
			Replicas:   16,
			Services:   40,
			ShmSizeMiB: 30720,
		},

		tenantID: tenantId,
		// fqdn:    "defang-lionello-alb-770995209.us-west-2.elb.amazonaws.com", // FIXME: grab these from the AWS API or outputs
		// privateLbIps:  nil,                                                 // TODO: grab these from the AWS API or outputs
		// publicNatIps:  nil,                                                 // TODO: grab these from the AWS API or outputs
	}
}

func (b *byocAws) setUp(ctx context.Context) error {
	if b.setupDone {
		return nil
	}
	// TODO: can we stick to the vanilla pulumi-nodejs image?
	if err := b.driver.SetUp(ctx, "docker.io/defangio/cd:"+cdVersion, 512_000_000, "linux/amd64"); err != nil {
		return annotateAwsError(err)
	}
	b.setupDone = true
	return nil
}

func (b *byocAws) Deploy(ctx context.Context, req *v1.DeployRequest) (*v1.DeployResponse, error) {
	etag := pkg.RandomID()
	if len(req.Services) > b.quota.Services {
		return nil, errors.New("maximum number of services reached")
	}
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
		// TODO: consider making this a proper Data URL: "data:application/protobuf;base64,abcd…"
	} else {
		url, err := b.driver.CreateUploadURL(ctx, etag)
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
			return nil, fmt.Errorf("unexpected status code during upload: %s", resp.Status)
		}
		payloadString = http.RemoveQueryParam(url)
		// FIXME: this code path didn't work
	}

	if err := b.setUp(ctx); err != nil {
		return nil, err
	}
	taskArn, err := b.runCdTask(ctx, "npm", "start", "up", payloadString)
	if err != nil {
		return nil, err
	}

	b.cdTasks[etag] = taskArn
	return &v1.DeployResponse{
		Services: serviceInfos,
		Etag:     etag,
	}, nil
}

func (b byocAws) GetStatus(ctx context.Context) (*v1.Status, error) {
	return &v1.Status{
		Version: cdVersion,
	}, nil
}

func (b byocAws) WhoAmI(ctx context.Context) (*v1.WhoAmIResponse, error) {
	// Use STS to get the account ID
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, annotateAwsError(err)
	}
	identity, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, annotateAwsError(err)
	}
	return &v1.WhoAmIResponse{
		Tenant:  b.tenantID,
		Region:  cfg.Region,
		Account: *identity.Account,
	}, nil
}

func (byocAws) GetVersion(context.Context) (*v1.Version, error) {
	return &v1.Version{Fabric: cdVersion}, nil
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
	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("service %q not found", s.Name))
}

func (b *byocAws) runCdTask(ctx context.Context, cmd ...string) (awsecs.TaskArn, error) {
	region := b.driver.Region // TODO: this should be the destination region, not the CD region; make customizable
	env := map[string]string{
		// "AWS_REGION":               region.String(), should be set by ECS (because of CD task role)
		"DEFANG_DEBUG":             os.Getenv("DEFANG_DEBUG"), // TODO: use the global DoDebug flag
		"DEFANG_ORG":               b.tenantID,
		"DOMAIN":                   b.customDomain,
		"PRIVATE_DOMAIN":           b.privateDomain,
		"PROJECT":                  projectName,
		"PULUMI_BACKEND_URL":       fmt.Sprintf(`s3://%s?region=%s&awssdk=v2`, b.driver.BucketName, region), // TODO: add a way to override bucket
		"PULUMI_CONFIG_PASSPHRASE": pkg.Getenv("PULUMI_CONFIG_PASSPHRASE", "asdf"),                          // TODO: make customizable
		"STACK":                    b.pulumiStack,
	}
	return b.driver.Run(ctx, env, cmd...)
}

func (b *byocAws) Delete(ctx context.Context, req *v1.DeleteRequest) (*v1.DeleteResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}
	// FIXME: this should only delete the services that are specified in the request, not all
	taskArn, err := b.runCdTask(ctx, "npm", "start", "up", "")
	if err != nil {
		return nil, annotateAwsError(err)
	}
	etag := awsecs.GetTaskID(taskArn) // TODO: this is the CD task ID, not the etag
	b.cdTasks[etag] = taskArn
	return &v1.DeleteResponse{Etag: etag}, nil
}

// stack returns a stack-qualified name, like the Pulumi TS function `stack`
func (b *byocAws) stack(name string) string {
	return projectName + "-" + b.pulumiStack + "-" + name
}

func (b *byocAws) getClusterNames() []string {
	// This should match the naming in pulumi/ecs/common.ts
	return []string{
		b.stack("cluster"),
		b.stack("gpu-cluster"),
	}
}

func (b byocAws) GetServices(ctx context.Context) (*v1.ListServicesResponse, error) {
	var maxResults int32 = 100 // the maximum allowed by AWS
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, annotateAwsError(err)
	}
	clusters := make(map[string][]string)
	ecsClient := ecs.NewFromConfig(cfg)
	for _, clusterName := range b.getClusterNames() {
		serviceArns, err := ecsClient.ListServices(ctx, &ecs.ListServicesInput{
			Cluster:    &clusterName,
			MaxResults: &maxResults, // TODO: handle pagination
		})
		if err != nil {
			var notFound *ecsTypes.ClusterNotFoundException
			if errors.As(err, &notFound) {
				continue
			}
			return nil, annotateAwsError(err)
		}
		clusters[clusterName] = serviceArns.ServiceArns
	}
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
			return nil, annotateAwsError(err)
		}
		for _, service := range dso.Services {
			// Check whether this is indeed a service we want to manage
			fqn := strings.SplitN(getQualifiedNameFromEcsName(*service.ServiceName), ".", 2)
			if len(fqn) != 2 {
				continue
			}
			// TODO: get the service definition from the task definition or tags
			serviceInfos = append(serviceInfos, &v1.ServiceInfo{
				Service: &v1.Service{
					Name: fqn[1],
				},
			})
		}
	}
	return &v1.ListServicesResponse{Services: serviceInfos}, nil
}

func getQualifiedNameFromEcsName(ecsService string) qualifiedName {
	// HACK: Pulumi adds a random 8-char suffix to the service name, so we need to strip it off.
	if len(ecsService) < 10 || ecsService[len(ecsService)-8] != '-' {
		return ""
	}
	serviceName := ecsService[:len(ecsService)-8]

	// Replace the first underscore to get the FQN.
	return qualifiedName(strings.Replace(serviceName, "_", ".", 1))
}

// annotateAwsError translates the AWS error to an error code the CLI client understands
func annotateAwsError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "get credentials:") {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	if aws.IsParameterNotFoundError(err) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	return err
}

func (b byocAws) PutSecret(ctx context.Context, secret *v1.SecretValue) error {
	if !pkg.IsValidSecretName(secret.Name) {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret name; must be alphanumeric or _, cannot start with a number: %q", secret.Name))
	}
	var err error
	prefix := b.tenantID + "."
	fqn := prefix + secret.Name
	if secret.Value == "" {
		err = b.driver.DeleteSecret(ctx, fqn)
	} else {
		err = b.driver.PutSecret(ctx, fqn, secret.Value)
	}
	return annotateAwsError(err)
}

func (b byocAws) ListSecrets(ctx context.Context) (*v1.Secrets, error) {
	awsSecrets, err := b.driver.ListSecretsByPrefix(ctx, b.tenantID)
	if err != nil {
		return nil, err
	}
	prefix := b.tenantID + "."
	secrets := make([]string, len(awsSecrets))
	for i, secret := range awsSecrets {
		secrets[i] = strings.TrimPrefix(secret, prefix)
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
	cancelTaskCh func()
	err          error
	errCh        <-chan error
	etag         string
	response     *v1.TailResponse
	service      string
	stream       awsecs.EventStream
	taskCh       <-chan error
}

var _ ServerStream[v1.TailResponse] = (*byocServerStream)(nil)

func (bs *byocServerStream) Close() error {
	if bs.cancelTaskCh != nil {
		bs.cancelTaskCh()
	}
	return bs.stream.Close()
}

func (bs *byocServerStream) Err() error {
	if bs.err == io.EOF {
		return nil // same as the original gRPC/connect server stream
	}
	return annotateAwsError(bs.err)
}

func (bs *byocServerStream) Msg() *v1.TailResponse {
	return bs.response
}

type hasErrCh interface {
	Errs() <-chan error
}

func (bs *byocServerStream) Receive() bool {
	select {
	case e := <-bs.stream.Events(): // blocking
		events, err := awsecs.GetLogEvents(e)
		if err != nil {
			bs.err = err
			return false
		}
		bs.response = &v1.TailResponse{}
		if len(events) == 0 {
			// The original gRPC/connect server stream would never send an empty response.
			// We could loop around the select, but returning an empty response updates the spinner.
			return true
		}
		var record logs.FirelensMessage
		parseFirelensRecords := false
		// Get the Etag/Host/Service from the first event (should be the same for all events in this batch)
		event := events[0]
		if strings.Contains(*event.LogGroupIdentifier, ":"+cdTaskPrefix) {
			// These events are from the CD task; detect the progress dots
			if len(events) == 1 && *event.Message == "." || *event.Message == "\033[38;5;3m.\033[0m" {
				// This is a progress dot; return an empty response
				return true
			}
			bs.response.Etag = bs.etag // FIXME: this would show all deployments, not just the one we're interested in
			bs.response.Host = "pulumi"
			bs.response.Service = "cd"
		} else if parts := strings.Split(*event.LogStreamName, "/"); len(parts) == 3 {
			// These events are from a awslogs service task: tenant/service_etag/taskID
			bs.response.Host = parts[2] // TODO: figure out hostname/IP
			parts = strings.Split(parts[1], "_")
			if len(parts) != 2 {
				// ignore sidecar logs (like route53-sidecar)
				return true
			}
			service, etag := parts[0], parts[1]
			bs.response.Etag = etag
			bs.response.Service = service
		} else if strings.Contains(*event.LogStreamName, "-firelens-") {
			// These events are from the Firelens sidecar; try to parse the JSON
			if err := json.Unmarshal([]byte(*event.Message), &record); err == nil {
				bs.response.Etag = record.Etag
				bs.response.Host = record.Host
				bs.response.Service = record.ContainerName // TODO: could be service_etag
				parseFirelensRecords = true
			}
		}
		if bs.etag != "" && bs.etag != bs.response.Etag {
			return true // TODO: filter these out using the AWS StartLiveTail API
		}
		if bs.service != "" && bs.service != bs.response.Service {
			return true // TODO: filter these out using the AWS StartLiveTail API
		}
		entries := make([]*v1.LogEntry, len(events))
		for i, event := range events {
			stderr := false //  TODO: detect somehow from source
			message := *event.Message
			if parseFirelensRecords {
				if err := json.Unmarshal([]byte(message), &record); err == nil {
					message = record.Log
					if record.ContainerName == "kaniko" {
						stderr = isLogrusError(message)
					} else {
						stderr = record.Source == logs.SourceStderr
					}
				}
			}
			entries[i] = &v1.LogEntry{
				Message:   message,
				Stderr:    stderr,
				Timestamp: timestamppb.New(time.UnixMilli(*event.Timestamp)),
			}
		}
		bs.response.Entries = entries
		return true

	case err := <-bs.errCh: // blocking (if not nil)
		bs.err = err
		return false

	case err := <-bs.taskCh: // blocking (if not nil)
		bs.err = err
		return false
	}
}

// Copied from server/fabric.go
func isLogrusError(message string) bool {
	message = pkg.StripAnsi(message)
	switch message[:pkg.Min(len(message), 4)] {
	case "WARN", "ERRO", "FATA", "PANI":
		return true // always show
	case "", ".", "INFO", "TRAC", "DEBU":
		return false // only shown with --verbose
	default:
		return true // show by default (likely Dockerfile errors)
	}
}

func (b *byocAws) Tail(ctx context.Context, req *v1.TailRequest) (ServerStream[v1.TailResponse], error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}
	etag := req.Etag
	// if etag == "" && req.Service == "cd" {
	// 	etag = awsecs.GetTaskID(b.cdTaskArn); TODO: find the last CD task
	// }
	// How to tail multiple tasks/services at once?
	//  * No Etag, no service:	tail all tasks/services
	//  * Etag, no service: 	tail all tasks/services with that Etag
	//  * No Etag, service:		tail all tasks/services with that service name
	//  * Etag, service:		tail that task/service
	var err error
	var cdTaskArn awsecs.TaskArn
	var eventStream awsecs.EventStream
	if etag != "" && !pkg.IsValidRandomID(etag) {
		// Assume "etag" is the CD task ID
		eventStream, err = b.driver.TailTask(ctx, etag)
		cdTaskArn, _ = b.driver.GetTaskArn(etag)
		etag = "" // no need to filter by etag
	} else {
		logGroupName := b.stack("kaniko") // TODO: rename this, but must match pulumi/index.ts
		logGroupID := b.driver.MakeARN("logs", "log-group:"+logGroupName)
		eventStream, err = awsecs.TailLogGroups(ctx, b.driver.LogGroupARN, logGroupID)
		cdTaskArn = b.cdTasks[etag]
	}
	if err != nil {
		return nil, annotateAwsError(err)
	}
	// if es, err := awsecs.Query(ctx, b.driver.LogGroupARN, req.Since.AsTime(), time.Now()); err == nil {
	// 	for _, e := range es {
	// 		println(*e.Message)
	// 	}
	// }
	var errCh <-chan error
	if errch, ok := eventStream.(hasErrCh); ok {
		errCh = errch.Errs()
	}
	var taskch <-chan error
	var cancel func()
	if cdTaskArn != nil {
		taskch, cancel = awsecs.TaskStatusCh(cdTaskArn, 3*time.Second) // check every 3 seconds
	}
	return &byocServerStream{
		cancelTaskCh: cancel,
		errCh:        errCh,
		etag:         etag,
		service:      req.Service,
		stream:       eventStream,
		taskCh:       taskch,
	}, nil
}

// This functions was copied from Fabric controller and slightly modified to work with BYOC
func (b byocAws) update(ctx context.Context, service *v1.Service) (*v1.ServiceInfo, error) {
	if err := b.quota.Validate(service); err != nil {
		return nil, err
	}

	// Check to make sure all required secrets are present in the secrets store
	if missing := b.checkForMissingSecrets(ctx, service.Secrets, b.tenantID); missing != nil {
		return nil, fmt.Errorf("missing secret %s", missing) // retryable CodeFailedPrecondition
	}
	si := &v1.ServiceInfo{
		Service: service,
		Project: b.tenantID,     // was: tenant
		Etag:    pkg.RandomID(), // TODO: could be hash for dedup/idempotency
	}

	hasHost := false
	hasIngress := false
	fqn := newQualifiedName(b.tenantID, service.Name)
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
			log.Printf("error looking up CNAME %q: %v\n", service.Domainname, err) // TODO: avoid `log` in CLI
			// Do not expose the error to the client, but fail the request with FailedPrecondition
		}
		if strings.TrimSuffix(cname, ".") != si.PublicFqdn {
			log.Printf("CNAME %q does not point to %q\n", service.Domainname, si.PublicFqdn) // TODO: avoid `log` in CLI
			// return nil, fmt.Errorf("CNAME %q does not point to %q", service.Domainname, si.PublicFqdn)); FIXME: circular dependency // CodeFailedPrecondition
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

// This functions was copied from Fabric controller and slightly modified to work with BYOC
func (b byocAws) checkForMissingSecrets(ctx context.Context, secrets []*v1.Secret, tenantId string) *v1.Secret {
	if len(secrets) == 1 {
		// Avoid fetching the list of secrets from AWS by only checking the one we need
		fqn := newQualifiedName(tenantId, secrets[0].Source)
		found, err := b.driver.IsValidSecret(ctx, fqn)
		if err != nil {
			log.Printf("error checking secret: %v\n", err) // TODO: avoid `log` in CLI
		}
		if !found {
			return secrets[0]
		}
	} else if len(secrets) > 1 {
		// Avoid multiple calls to AWS by sorting the list and then doing a binary search
		sorted, err := b.driver.ListSecretsByPrefix(ctx, b.tenantID)
		if err != nil {
			log.Println("error listing secrets:", err) // TODO: avoid `log` in CLI
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

// This functions was copied from Fabric controller and slightly modified to work with BYOC
func (b byocAws) getEndpoint(fqn qualifiedName, port *v1.Port) string {
	safeFqn := dnsSafe(fqn)
	if port.Mode == v1.Mode_HOST {
		return fmt.Sprintf("%s.%s:%d", safeFqn, b.privateDomain, port.Target)
	} else {
		if b.customDomain == "" {
			return ":443" // placeholder for the public ALB/distribution
		}
		return fmt.Sprintf("%s--%d.%s", safeFqn, port.Target, b.customDomain)
	}
}

// This functions was copied from Fabric controller and slightly modified to work with BYOC
func (b byocAws) getFqdn(fqn qualifiedName, public bool) string {
	safeFqn := dnsSafe(fqn)
	if public {
		if b.customDomain == "" {
			return "" //b.fqdn
		}
		return fmt.Sprintf("%s.%s", safeFqn, b.customDomain)
	} else {
		return fmt.Sprintf("%s.%s", safeFqn, b.privateDomain)
	}
}

// This functions was copied from Fabric controller and slightly modified to work with BYOC
func dnsSafe(fqn qualifiedName) string {
	return strings.ReplaceAll(string(fqn), ".", "-")
}

func (b *byocAws) Destroy(ctx context.Context) error {
	if err := b.setUp(ctx); err != nil {
		return err
	}
	return b.driver.TearDown(ctx)
}

func (b *byocAws) BootstrapCommand(ctx context.Context, command string) error {
	if err := b.setUp(ctx); err != nil {
		return err
	}
	if _, err := b.runCdTask(ctx, "npm", "start", command); err != nil {
		return err
	}
	return nil
}
