package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53Types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/ptr"
	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/aws"
	awsecs "github.com/defang-io/defang/src/pkg/aws/ecs"
	"github.com/defang-io/defang/src/pkg/aws/ecs/cfn"
	"github.com/defang-io/defang/src/pkg/http"
	"github.com/defang-io/defang/src/pkg/logs"
	"github.com/defang-io/defang/src/pkg/quota"
	"github.com/defang-io/defang/src/pkg/types"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	cdTaskPrefix = "defang-cd" // WARNING: renaming this practically deletes the Pulumi state
	defangPrefix = "Defang"    // prefix for all resources created by Defang
)

var (
	// Changing this will cause issues if two clients with different versions are using the same account
	cdImage = pkg.Getenv("DEFANG_CD_IMAGE", "public.ecr.aws/defang-io/cd:public-beta")
)

type byocAws struct {
	*GrpcClient

	cdTasks                 map[string]awsecs.TaskArn
	customDomain            string
	driver                  *cfn.AwsEcs
	privateDomain           string
	privateLbIps            []string
	publicNatIps            []string
	pulumiProject           string
	pulumiStack             string
	quota                   quota.Quotas
	setupDone               bool
	tenantID                string
	shouldDelegateSubdomain bool
}

type Warning interface {
	Error() string
	Warning() string
}

type WarningError string

func (w WarningError) Error() string {
	return string(w)
}

func (w WarningError) Warning() string {
	return string(w)
}

type Warnings []Warning

func (w Warnings) Error() string {
	var buf strings.Builder
	for _, warning := range w {
		buf.WriteString(warning.Warning())
		buf.WriteByte('\n')
	}
	return buf.String()
}

var _ Client = (*byocAws)(nil)

func NewByocAWS(tenantId types.TenantID, project string, defClient *GrpcClient) *byocAws {
	return &byocAws{
		GrpcClient:    defClient,
		cdTasks:       make(map[string]awsecs.TaskArn),
		customDomain:  "",
		driver:        cfn.New(cdTaskPrefix, aws.Region("")),  // default region
		privateDomain: strings.ToLower(project + ".internal"), // must match the logic in ecs/common.ts
		pulumiProject: project,                                // TODO: multi-project support
		pulumiStack:   "beta",                                 // TODO: make customizable
		quota: quota.Quotas{
			// These serve mostly to pevent fat-finger errors in the CLI or Compose files
			Cpus:       16,
			Gpus:       8,
			MemoryMiB:  65536,
			Replicas:   16,
			Services:   40,
			ShmSizeMiB: 30720,
		},
		tenantID: string(tenantId),
		// privateLbIps:  nil,                                                 // TODO: grab these from the AWS API or outputs
		// publicNatIps:  nil,                                                 // TODO: grab these from the AWS API or outputs
	}
}

func (b *byocAws) setUp(ctx context.Context) error {
	if b.setupDone {
		return nil
	}
	cdTaskName := cdTaskPrefix
	tasks := []types.Task{
		{
			Image:     "pulumi/pulumi:latest",
			Name:      awsecs.ContainerName,
			Memory:    4 * 512_000_000, // 512 MiB
			Essential: ptr.Bool(true),
			VolumesFrom: []string{
				cdTaskName,
			},
			EntryPoint: []string{"node", "lib/index.js"},
		},
		{
			Image:     cdImage,
			Name:      cdTaskName,
			Essential: ptr.Bool(false),
			Volumes: []types.TaskVolume{
				{
					Source:   "pulumi-plugins",
					Target:   "/root/.pulumi/plugins",
					ReadOnly: true,
				},
				{
					Source:   "cd",
					Target:   "/app",
					ReadOnly: true,
				},
			},
		},
	}
	if err := b.driver.SetUp(ctx, tasks); err != nil {
		return annotateAwsError(err)
	}

	if b.customDomain == "" {
		domain, err := b.GetDelegateSubdomainZone(ctx, &v1.GetDelegateSubdomainZoneRequest{Project: b.pulumiProject})
		if err != nil {
			// return err; FIXME: ignore this error for now
		} else {
			b.customDomain = strings.ToLower(domain.Zone) // HACK: this should be DnsSafe
			b.shouldDelegateSubdomain = true
		}
	}

	b.setupDone = true
	return nil
}

func (b *byocAws) Deploy(ctx context.Context, req *v1.DeployRequest) (*v1.DeployResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	etag := pkg.RandomID()
	if len(req.Services) > b.quota.Services {
		return nil, errors.New("maximum number of services reached")
	}
	serviceInfos := []*v1.ServiceInfo{}
	var warnings Warnings
	for _, service := range req.Services {
		serviceInfo, err := b.update(ctx, service)
		var warning Warning
		if errors.As(err, &warning) && warning != nil {
			warnings = append(warnings, warning)
		} else if err != nil {
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

	if b.shouldDelegateSubdomain {
		if _, err := b.delegateSubdomain(ctx); err != nil {
			return nil, err
		}
	}
	taskArn, err := b.runCdCommand(ctx, "up", payloadString)
	if err != nil {
		return nil, err
	}
	b.cdTasks[etag] = taskArn

	return &v1.DeployResponse{
		Services: serviceInfos,
		Etag:     etag,
	}, warnings
}

func (b byocAws) FindZone(ctx context.Context, domain, role string) (string, error) {
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return "", annotateAwsError(err)
	}

	if role != "" {
		stsClient := sts.NewFromConfig(cfg)
		creds := stscreds.NewAssumeRoleProvider(stsClient, role)
		cfg.Credentials = awssdk.NewCredentialsCache(creds)
	}

	r53Client := route53.NewFromConfig(cfg)

	domain = strings.TrimSuffix(domain, ".")
	domain = strings.ToLower(domain)
	for {
		zoneId, err := aws.GetZoneIdFromDomain(ctx, domain, r53Client)
		if errors.Is(err, aws.ErrNoZoneFound) {
			if strings.Count(domain, ".") <= 1 {
				return "", nil
			}
			domain = domain[strings.Index(domain, ".")+1:]
			continue
		} else if err != nil {
			return "", err
		}
		return zoneId, nil
	}
}

func (b byocAws) delegateSubdomain(ctx context.Context) (string, error) {
	if b.customDomain == "" {
		return "", errors.New("custom domain not set")
	}
	domain := b.customDomain
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return "", annotateAwsError(err)
	}
	r53Client := route53.NewFromConfig(cfg)

	zoneId, err := aws.GetZoneIdFromDomain(ctx, domain, r53Client)
	if errors.Is(err, aws.ErrNoZoneFound) {
		zoneId, err = aws.CreateZone(ctx, domain, r53Client)
		if err != nil {
			return "", annotateAwsError(err)
		}
	} else if err != nil {
		return "", annotateAwsError(err)
	}

	// Get the NS records for the subdomain zone and call DelegateSubdomainZone again
	nsServers, err := aws.GetRecordsValue(ctx, zoneId, domain, r53Types.RRTypeNs, r53Client)
	if err != nil {
		return "", annotateAwsError(err)
	}
	if len(nsServers) == 0 {
		return "", errors.New("no NS records found for the subdomain zone")
	}

	req := &v1.DelegateSubdomainZoneRequest{NameServerRecords: nsServers, Project: b.pulumiProject}
	resp, err := b.DelegateSubdomainZone(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Zone, nil
}

func (b byocAws) WhoAmI(ctx context.Context) (*v1.WhoAmIResponse, error) {
	if _, err := b.GrpcClient.WhoAmI(ctx); err != nil {
		return nil, err
	}

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
	cdVersion := cdImage[strings.LastIndex(cdImage, ":")+1:]
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

func (b *byocAws) environment() map[string]string {
	region := b.driver.Region // TODO: this should be the destination region, not the CD region; make customizable
	return map[string]string{
		// "AWS_REGION":               region.String(), should be set by ECS (because of CD task role)
		"DEFANG_PREFIX":            defangPrefix,
		"DEFANG_DEBUG":             os.Getenv("DEFANG_DEBUG"), // TODO: use the global DoDebug flag
		"DEFANG_ORG":               b.tenantID,
		"DOMAIN":                   b.customDomain,
		"PRIVATE_DOMAIN":           b.privateDomain,
		"PROJECT":                  b.pulumiProject,
		"PULUMI_BACKEND_URL":       fmt.Sprintf(`s3://%s?region=%s&awssdk=v2`, b.driver.BucketName, region), // TODO: add a way to override bucket
		"PULUMI_CONFIG_PASSPHRASE": pkg.Getenv("PULUMI_CONFIG_PASSPHRASE", "asdf"),                          // TODO: make customizable
		"STACK":                    b.pulumiStack,
	}
}

func (b *byocAws) runCdCommand(ctx context.Context, cmd ...string) (awsecs.TaskArn, error) {
	env := b.environment()
	return b.driver.Run(ctx, env, cmd...)
}

func (b *byocAws) Delete(ctx context.Context, req *v1.DeleteRequest) (*v1.DeleteResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}
	// FIXME: this should only delete the services that are specified in the request, not all
	taskArn, err := b.runCdCommand(ctx, "up", "")
	if err != nil {
		return nil, annotateAwsError(err)
	}
	etag := awsecs.GetTaskID(taskArn) // TODO: this is the CD task ID, not the etag
	b.cdTasks[etag] = taskArn
	return &v1.DeleteResponse{Etag: etag}, nil
}

// stack returns a stack-qualified name, like the Pulumi TS function `stack`
func (b *byocAws) stack(name string) string {
	return fmt.Sprintf("%s-%s-%s-%s", defangPrefix, b.pulumiProject, b.pulumiStack, name) // same as shared/common.ts
}

func (b *byocAws) stackDir(name string) string {
	return fmt.Sprintf("/%s/%s/%s/%s", defangPrefix, b.pulumiProject, b.pulumiStack, name) // same as shared/common.ts
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

func (b byocAws) getSecretID(name string) string {
	return fmt.Sprintf("/%s/%s/%s/%s", defangPrefix, b.pulumiProject, b.pulumiStack, name) // same as defang_service.ts
}

func (b byocAws) PutSecret(ctx context.Context, secret *v1.SecretValue) error {
	if !pkg.IsValidSecretName(secret.Name) {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret name; must be alphanumeric or _, cannot start with a number: %q", secret.Name))
	}
	fqn := b.getSecretID(secret.Name)
	var err error
	if secret.Value == "" {
		err = b.driver.DeleteSecret(ctx, fqn)
	} else {
		err = b.driver.PutSecret(ctx, fqn, secret.Value)
	}
	return annotateAwsError(err)
}

func (b byocAws) ListSecrets(ctx context.Context) (*v1.Secrets, error) {
	prefix := b.getSecretID("")
	awsSecrets, err := b.driver.ListSecretsByPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
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
			// These events are from the CD task; detect stdout/stderr
			bs.response.Etag = bs.etag // FIXME: this would show all deployments, not just the one we're interested in
			bs.response.Host = "pulumi"
			bs.response.Service = "cd"
		} else if parts := strings.Split(*event.LogStreamName, "/"); len(parts) == 3 {
			// These events are from a awslogs service task: tenant/service_etag/taskID
			bs.response.Host = parts[2] // TODO: figure out hostname/IP
			parts = strings.Split(parts[1], "_")
			if len(parts) != 2 || !pkg.IsValidRandomID(parts[1]) {
				// ignore sidecar logs (like route53-sidecar or fluentbit)
				return true
			}
			service, etag := parts[0], parts[1]
			bs.response.Etag = etag
			bs.response.Service = service
		} else if strings.Contains(*event.LogStreamName, "-firelens-") {
			// These events are from the Firelens sidecar; try to parse the JSON
			if err := json.Unmarshal([]byte(*event.Message), &record); err == nil {
				bs.response.Etag = record.Etag
				bs.response.Host = record.Host             // TODO: use "kaniko" for kaniko logs
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
						stderr = logs.IsLogrusError(message)
					} else {
						stderr = record.Source == logs.SourceStderr
					}
				}
			} else if bs.response.Service == "cd" && strings.HasPrefix(message, " ** ") {
				stderr = true
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
		return false // TODO: make sure we got all the logs from the task
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
		eventStream, err = b.driver.TailTaskID(ctx, etag)
		cdTaskArn, _ = b.driver.GetTaskArn(etag)
		etag = "" // no need to filter by etag
	} else {
		// Tail CD, kaniko, and all services
		kanikoLogGroup := b.driver.MakeARN("logs", "log-group:"+b.stackDir("kaniko"))     // must match logic in ecs/common.ts
		servicesLogGroup := b.driver.MakeARN("logs", "log-group:"+b.stackDir("logGroup")) // must match logic in ecs/common.ts
		eventStream, err = awsecs.TailLogGroups(ctx, b.driver.LogGroupARN, kanikoLogGroup, servicesLogGroup)
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

	taskch := make(chan error)
	var cancel func()
	if cdTaskArn != nil {
		ctx, cancel = context.WithCancel(ctx)
		go func() {
			taskch <- awsecs.WaitForTask(ctx, cdTaskArn, 3*time.Second)
		}()
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

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b byocAws) update(ctx context.Context, service *v1.Service) (*v1.ServiceInfo, error) {
	if err := b.quota.Validate(service); err != nil {
		return nil, err
	}

	// Check to make sure all required secrets are present in the secrets store
	missing, err := b.checkForMissingSecrets(ctx, service.Secrets, b.tenantID)
	if err != nil {
		return nil, err
	}
	if missing != nil {
		return nil, fmt.Errorf("missing secret %s", missing) // retryable CodeFailedPrecondition
	}

	si := &v1.ServiceInfo{
		Service: service,
		Project: b.pulumiProject, // was: tenant
		Etag:    pkg.RandomID(),  // TODO: could be hash for dedup/idempotency
	}

	hasHost := false
	hasIngress := false
	fqn := service.Name //newQualifiedName(b.tenantID, service.Name)
	for _, port := range service.Ports {
		hasIngress = hasIngress || port.Mode == v1.Mode_INGRESS
		hasHost = hasHost || port.Mode == v1.Mode_HOST
		si.Endpoints = append(si.Endpoints, b.getEndpoint(fqn, port))
	}
	if hasIngress {
		si.LbIps = b.privateLbIps // only set LB IPs if there are ingress ports
		si.PublicFqdn = b.getPublicFqdn(fqn)
	}
	if hasHost {
		si.PrivateFqdn = b.getPrivateFqdn(fqn)
	}

	var warning Warning
	if service.Domainname != "" {
		if !hasIngress {
			return nil, errors.New("domainname requires at least one ingress port") // retryable CodeFailedPrecondition
		}
		// Do a DNS lookup for Domainname and confirm it's indeed a CNAME to the service's public FQDN
		cname, err := net.LookupCNAME(service.Domainname)
		if err != nil {
			warning = WarningError(fmt.Sprintf("error looking up CNAME %q: %v", service.Domainname, err))
		}
		if strings.TrimSuffix(cname, ".") != si.PublicFqdn {
			zoneId, err := b.FindZone(ctx, service.Domainname, service.DnsRole)
			if err != nil {
				return nil, err
			}
			if zoneId != "" {
				si.ZoneId = zoneId
			} else {
				warning = WarningError(fmt.Sprintf("CNAME %q does not point to %q and no route53 zone managing domain was found", service.Domainname, si.PublicFqdn))
			}
		}
	}

	si.NatIps = b.publicNatIps // TODO: even internal services use NAT now
	si.Status = "UPDATE_QUEUED"
	if si.Service.Build != nil {
		si.Status = "BUILD_QUEUED" // in SaaS, this gets overwritten by the ECS events for "kaniko"
	}
	return si, warning
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b byocAws) checkForMissingSecrets(ctx context.Context, secrets []*v1.Secret, tenantId string) (*v1.Secret, error) {
	prefix := b.getSecretID("")
	sorted, err := b.driver.ListSecretsByPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
	for _, secret := range secrets {
		fqn := b.getSecretID(secret.Source)
		i := sort.Search(len(sorted), func(i int) bool {
			return sorted[i] >= fqn
		})
		if i >= len(sorted) || sorted[i] != fqn {
			return secret, nil // secret not found
		}
	}
	return nil, nil // all secrets found (or none specified)
}

type qualifiedName = string // legacy

// This function was copied from Fabric controller and slightly modified to work with BYOC
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

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b byocAws) getPublicFqdn(fqn qualifiedName) string {
	safeFqn := dnsSafe(fqn)
	if b.customDomain == "" {
		return "" //b.fqdn
	}
	return fmt.Sprintf("%s.%s", safeFqn, b.customDomain)
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b byocAws) getPrivateFqdn(fqn qualifiedName) string {
	safeFqn := dnsSafe(fqn)
	return fmt.Sprintf("%s.%s", safeFqn, b.privateDomain)
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func dnsSafe(fqn qualifiedName) string {
	return strings.ReplaceAll(strings.ToLower(string(fqn)), ".", "-")
}

func (b *byocAws) TearDown(ctx context.Context) error {
	return b.driver.TearDown(ctx)
}

func (b *byocAws) BootstrapCommand(ctx context.Context, command string) (string, error) {
	if err := b.setUp(ctx); err != nil {
		return "", err
	}
	cdTaskArn, err := b.runCdCommand(ctx, command)
	if err != nil || cdTaskArn == nil {
		return "", annotateAwsError(err)
	}
	return awsecs.GetTaskID(cdTaskArn), nil
}

func (b *byocAws) Destroy(ctx context.Context) (string, error) {
	return b.BootstrapCommand(ctx, "down")
}
