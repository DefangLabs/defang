package clouds

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	aws2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	ecs2 "github.com/aws/aws-sdk-go-v2/service/ecs"
	types3 "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	types2 "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/ptr"
	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/clouds/aws"
	"github.com/defang-io/defang/src/pkg/clouds/aws/ecs"
	"github.com/defang-io/defang/src/pkg/clouds/aws/ecs/cfn"
	"github.com/defang-io/defang/src/pkg/http"
	"github.com/defang-io/defang/src/pkg/logs"
	"github.com/defang-io/defang/src/pkg/quota"
	"github.com/defang-io/defang/src/pkg/types"
	"github.com/defang-io/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"time"
)

type ByocAws struct {
	*client.GrpcClient

	cdTasks                 map[string]ecs.TaskArn
	CustomDomain            string
	Driver                  *cfn.AwsEcs
	privateDomain           string
	privateLbIps            []string
	publicNatIps            []string
	pulumiProject           string
	pulumiStack             string
	quota                   quota.Quotas
	setupDone               bool
	TenantID                string
	shouldDelegateSubdomain bool
}

func NewByocAWS(tenantId types.TenantID, project string, defClient *client.GrpcClient) *ByocAws {
	// Resource naming (stack/stackDir) requires a project name
	if project == "" {
		project = tenantId.String()
	}
	b := &ByocAws{
		GrpcClient:    defClient,
		cdTasks:       make(map[string]ecs.TaskArn),
		CustomDomain:  "",
		Driver:        cfn.New(CdTaskPrefix, aws.Region("")), // default region
		pulumiProject: project,                               // TODO: multi-project support
		pulumiStack:   "beta",                                // TODO: make customizable
		quota: quota.Quotas{
			// These serve mostly to pevent fat-finger errors in the CLI or Compose files
			Cpus:       16,
			Gpus:       8,
			MemoryMiB:  65536,
			Replicas:   16,
			Services:   40,
			ShmSizeMiB: 30720,
		},
		TenantID: string(tenantId),
		// privateLbIps:  nil,                                                 // TODO: grab these from the AWS API or outputs
		// publicNatIps:  nil,                                                 // TODO: grab these from the AWS API or outputs
	}
	b.privateDomain = b.getProjectDomain("internal")
	return b
}

func (b *ByocAws) setUp(ctx context.Context) error {
	if b.setupDone {
		return nil
	}
	cdTaskName := CdTaskPrefix
	containers := []types.Container{
		{
			Image:     "public.ecr.aws/pulumi/pulumi-nodejs:latest",
			Name:      ecs.ContainerName,
			Cpus:      0.5,
			Memory:    2048_000_000, // 2G
			Essential: ptr.Bool(true),
			VolumesFrom: []string{
				cdTaskName,
			},
			WorkDir:    ptr.String("/app"),
			DependsOn:  map[string]types.ContainerCondition{cdTaskName: "START"},
			EntryPoint: []string{"node", "lib/index.js"},
		},
		{
			Image:     CdImage,
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
	if err := b.Driver.SetUp(ctx, containers); err != nil {
		return annotateAwsError(err)
	}

	if b.CustomDomain == "" {
		domain, err := b.GetDelegateSubdomainZone(ctx)
		if err != nil {
			// return err; FIXME: ignore this error for now
		} else {
			b.CustomDomain = strings.ToLower(domain.Zone) // HACK: this should be DnsSafe
			b.shouldDelegateSubdomain = true
		}
	}

	b.setupDone = true
	return nil
}

func (b *ByocAws) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	etag := pkg.RandomID()
	if len(req.Services) > b.quota.Services {
		return nil, errors.New("maximum number of services reached")
	}
	serviceInfos := []*defangv1.ServiceInfo{}
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

	data, err := proto.Marshal(&defangv1.ListServicesResponse{
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
		url, err := b.Driver.CreateUploadURL(ctx, etag)
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

	return &defangv1.DeployResponse{
		Services: serviceInfos,
		Etag:     etag,
	}, warnings
}

func (b ByocAws) findZone(ctx context.Context, domain, role string) (string, error) {
	cfg, err := b.Driver.LoadConfig(ctx)
	if err != nil {
		return "", annotateAwsError(err)
	}

	if role != "" {
		stsClient := sts.NewFromConfig(cfg)
		creds := stscreds.NewAssumeRoleProvider(stsClient, role)
		cfg.Credentials = aws2.NewCredentialsCache(creds)
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

func (b ByocAws) delegateSubdomain(ctx context.Context) (string, error) {
	if b.CustomDomain == "" {
		return "", errors.New("custom domain not set")
	}
	domain := b.CustomDomain
	cfg, err := b.Driver.LoadConfig(ctx)
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
	nsServers, err := aws.GetRecordsValue(ctx, zoneId, domain, types2.RRTypeNs, r53Client)
	if err != nil {
		return "", annotateAwsError(err)
	}
	if len(nsServers) == 0 {
		return "", errors.New("no NS records found for the subdomain zone")
	}

	req := &defangv1.DelegateSubdomainZoneRequest{NameServerRecords: nsServers}
	resp, err := b.DelegateSubdomainZone(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Zone, nil
}

func (b ByocAws) WhoAmI(ctx context.Context) (*defangv1.WhoAmIResponse, error) {
	if _, err := b.GrpcClient.WhoAmI(ctx); err != nil {
		return nil, err
	}

	// Use STS to get the account ID
	cfg, err := b.Driver.LoadConfig(ctx)
	if err != nil {
		return nil, annotateAwsError(err)
	}
	identity, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, annotateAwsError(err)
	}
	return &defangv1.WhoAmIResponse{
		Tenant:  b.TenantID,
		Region:  cfg.Region,
		Account: *identity.Account,
	}, nil
}

func (ByocAws) GetVersion(context.Context) (*defangv1.Version, error) {
	cdVersion := CdImage[strings.LastIndex(CdImage, ":")+1:]
	return &defangv1.Version{Fabric: cdVersion}, nil
}

func (b ByocAws) Get(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
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

func (b *ByocAws) environment() map[string]string {
	region := b.Driver.Region // TODO: this should be the destination region, not the CD region; make customizable
	return map[string]string{
		// "AWS_REGION":               region.String(), should be set by ECS (because of CD task role)
		"DEFANG_PREFIX":              DefangPrefix,
		"DEFANG_DEBUG":               os.Getenv("DEFANG_DEBUG"), // TODO: use the global DoDebug flag
		"DEFANG_ORG":                 b.TenantID,
		"DOMAIN":                     b.CustomDomain,
		"PRIVATE_DOMAIN":             b.privateDomain,
		"PROJECT":                    b.pulumiProject,
		"PULUMI_BACKEND_URL":         fmt.Sprintf(`s3://%s?region=%s&awssdk=v2`, b.Driver.BucketName, region), // TODO: add a way to override bucket
		"PULUMI_CONFIG_PASSPHRASE":   pkg.Getenv("PULUMI_CONFIG_PASSPHRASE", "asdf"),                          // TODO: make customizable
		"STACK":                      b.pulumiStack,
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",
		"PULUMI_SKIP_UPDATE_CHECK":   "true",
	}
}

func (b *ByocAws) runCdCommand(ctx context.Context, cmd ...string) (ecs.TaskArn, error) {
	env := b.environment()
	return b.Driver.Run(ctx, env, cmd...)
}

func (b *ByocAws) Delete(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}
	// FIXME: this should only delete the services that are specified in the request, not all
	taskArn, err := b.runCdCommand(ctx, "up", "")
	if err != nil {
		return nil, annotateAwsError(err)
	}
	etag := ecs.GetTaskID(taskArn) // TODO: this is the CD task ID, not the etag
	b.cdTasks[etag] = taskArn
	return &defangv1.DeleteResponse{Etag: etag}, nil
}

// stack returns a stack-qualified name, like the Pulumi TS function `stack`
func (b *ByocAws) stack(name string) string {
	return fmt.Sprintf("%s-%s-%s-%s", DefangPrefix, b.pulumiProject, b.pulumiStack, name) // same as shared/common.ts
}

func (b *ByocAws) stackDir(name string) string {
	return fmt.Sprintf("/%s/%s/%s/%s", DefangPrefix, b.pulumiProject, b.pulumiStack, name) // same as shared/common.ts
}

func (b *ByocAws) getClusterNames() []string {
	// This should match the naming in pulumi/ecs/common.ts
	return []string{
		b.stack("cluster"),
		b.stack("gpu-cluster"),
	}
}

func (b ByocAws) GetServices(ctx context.Context) (*defangv1.ListServicesResponse, error) {
	var maxResults int32 = 100 // the maximum allowed by AWS
	cfg, err := b.Driver.LoadConfig(ctx)
	if err != nil {
		return nil, annotateAwsError(err)
	}
	clusters := make(map[string][]string)
	ecsClient := ecs2.NewFromConfig(cfg)
	for _, clusterName := range b.getClusterNames() {
		serviceArns, err := ecsClient.ListServices(ctx, &ecs2.ListServicesInput{
			Cluster:    &clusterName,
			MaxResults: &maxResults, // TODO: handle pagination
		})
		if err != nil {
			var notFound *types3.ClusterNotFoundException
			if errors.As(err, &notFound) {
				continue
			}
			return nil, annotateAwsError(err)
		}
		clusters[clusterName] = serviceArns.ServiceArns
	}
	// Query services for each cluster
	serviceInfos := []*defangv1.ServiceInfo{}
	for cluster, serviceNames := range clusters {
		if len(serviceNames) == 0 {
			continue
		}
		dso, err := ecsClient.DescribeServices(ctx, &ecs2.DescribeServicesInput{
			Services: serviceNames,
			Cluster:  &cluster,
		})
		if err != nil {
			return nil, annotateAwsError(err)
		}
		for _, service := range dso.Services {
			// Check whether this is indeed a service we want to manage
			fqn := strings.Split(getQualifiedNameFromEcsName(*service.ServiceName), ".")
			if len(fqn) != 2 {
				continue
			}
			serviceInfo := &defangv1.ServiceInfo{
				CreatedAt: timestamppb.New(*service.CreatedAt),
				Project:   fqn[0],
				Service: &defangv1.Service{
					Name: fqn[1],
					Deploy: &defangv1.Deploy{
						Replicas: uint32(service.DesiredCount),
					},
				},
				Status: *service.Status,
			}
			// TODO: get the service definition from the task definition or tags
			for _, tag := range service.Tags {
				if *tag.Key == "etag" {
					serviceInfo.Etag = *tag.Value
					break
				}
			}
			serviceInfos = append(serviceInfos, serviceInfo)
		}
	}
	return &defangv1.ListServicesResponse{Services: serviceInfos}, nil
}

func (b ByocAws) getSecretID(name string) string {
	return fmt.Sprintf("/%s/%s/%s/%s", DefangPrefix, b.pulumiProject, b.pulumiStack, name) // same as defang_service.ts
}

func (b ByocAws) PutSecret(ctx context.Context, secret *defangv1.SecretValue) error {
	if !pkg.IsValidSecretName(secret.Name) {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret name; must be alphanumeric or _, cannot start with a number: %q", secret.Name))
	}
	fqn := b.getSecretID(secret.Name)
	err := b.Driver.PutSecret(ctx, fqn, secret.Value)
	return annotateAwsError(err)
}

func (b ByocAws) ListSecrets(ctx context.Context) (*defangv1.Secrets, error) {
	prefix := b.getSecretID("")
	awsSecrets, err := b.Driver.ListSecretsByPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
	secrets := make([]string, len(awsSecrets))
	for i, secret := range awsSecrets {
		secrets[i] = strings.TrimPrefix(secret, prefix)
	}
	return &defangv1.Secrets{Names: secrets}, nil
}

func (b *ByocAws) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	url, err := b.Driver.CreateUploadURL(ctx, req.Digest)
	if err != nil {
		return nil, err
	}
	return &defangv1.UploadURLResponse{
		Url: url,
	}, nil
}

func (b *ByocAws) Tail(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
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
	var taskArn ecs.TaskArn
	var eventStream ecs.EventStream
	if etag != "" && !pkg.IsValidRandomID(etag) {
		// Assume "etag" is a task ID
		eventStream, err = b.Driver.TailTaskID(ctx, etag)
		taskArn, _ = b.Driver.GetTaskArn(etag)
		etag = "" // no need to filter by etag
	} else {
		// Tail CD, kaniko, and all services
		kanikoTail := ecs.LogGroupInput{LogGroupARN: b.Driver.MakeARN("logs", "log-group:"+b.stackDir("builds"))} // must match logic in ecs/common.ts
		servicesTail := ecs.LogGroupInput{LogGroupARN: b.Driver.MakeARN("logs", "log-group:"+b.stackDir("logs"))} // must match logic in ecs/common.ts
		cdTail := ecs.LogGroupInput{LogGroupARN: b.Driver.LogGroupARN}
		taskArn = b.cdTasks[etag]
		if taskArn != nil {
			// Only tail the logstreams for the CD task
			cdTail.LogStreamNames = []string{ecs.GetLogStreamForTaskID(ecs.GetTaskID(taskArn))}
		}
		eventStream, err = ecs.TailLogGroups(ctx, cdTail, kanikoTail, servicesTail)
	}
	if err != nil {
		return nil, annotateAwsError(err)
	}
	// if es, err := awsecs.Query(ctx, b.Driver.LogGroupARN, req.Since.AsTime(), time.Now()); err == nil {
	// 	for _, e := range es {
	// 		println(*e.Message)
	// 	}
	// }
	var errCh <-chan error
	if errch, ok := eventStream.(hasErrCh); ok {
		errCh = errch.Errs()
	}

	taskch := make(chan error) // TODO: close?
	var cancel func()
	if taskArn != nil {
		ctx, cancel = context.WithCancel(ctx)
		go func() {
			taskch <- ecs.WaitForTask(ctx, taskArn, 3*time.Second)
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
func (b ByocAws) update(ctx context.Context, service *defangv1.Service) (*defangv1.ServiceInfo, error) {
	if err := b.quota.Validate(service); err != nil {
		return nil, err
	}

	// Check to make sure all required secrets are present in the secrets store
	missing, err := b.checkForMissingSecrets(ctx, service.Secrets)
	if err != nil {
		return nil, err
	}
	if missing != nil {
		return nil, fmt.Errorf("missing secret %s", missing) // retryable CodeFailedPrecondition
	}

	si := &defangv1.ServiceInfo{
		Service: service,
		Project: b.pulumiProject, // was: tenant
		Etag:    pkg.RandomID(),  // TODO: could be hash for dedup/idempotency
	}

	hasHost := false
	hasIngress := false
	fqn := service.Name //newQualifiedName(b.TenantID, service.Name)
	for _, port := range service.Ports {
		hasIngress = hasIngress || port.Mode == defangv1.Mode_INGRESS
		hasHost = hasHost || port.Mode == defangv1.Mode_HOST
		si.Endpoints = append(si.Endpoints, b.GetEndpoint(fqn, port))
	}
	if hasIngress {
		si.LbIps = b.privateLbIps // only set LB IPs if there are ingress ports
		si.PublicFqdn = b.GetPublicFqdn(fqn)
	}
	if hasHost {
		si.PrivateFqdn = b.GetPrivateFqdn(fqn)
	}

	var warning Warning
	if service.Domainname != "" {
		if !hasIngress {
			return nil, errors.New("domainname requires at least one ingress port") // retryable CodeFailedPrecondition
		}
		// Do a DNS lookup for Domainname and confirm it's indeed a CNAME to the service's public FQDN
		cname, _ := net.LookupCNAME(service.Domainname)
		if strings.TrimSuffix(cname, ".") != si.PublicFqdn {
			zoneId, err := b.findZone(ctx, service.Domainname, service.DnsRole)
			if err != nil {
				return nil, err
			}
			if zoneId != "" {
				si.ZoneId = zoneId
			} else {
				si.UseAcmeCert = true
				// TODO: We should add link to documentation on how the acme cert workflow works
				// TODO: Should we make this the default behavior or require the user to set a flag?
				warning = WarningError(fmt.Sprintf("CNAME %q does not point to %q and no route53 zone managing domain was found, a let's encrypt cert will be used on first visit to the http end point", service.Domainname, si.PublicFqdn))
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
func (b ByocAws) checkForMissingSecrets(ctx context.Context, secrets []*defangv1.Secret) (*defangv1.Secret, error) {
	prefix := b.getSecretID("")
	sorted, err := b.Driver.ListSecretsByPrefix(ctx, prefix)
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
func (b ByocAws) GetEndpoint(fqn qualifiedName, port *defangv1.Port) string {
	safeFqn := dnsSafe(fqn)
	if port.Mode == defangv1.Mode_HOST {
		return fmt.Sprintf("%s.%s:%d", safeFqn, b.privateDomain, port.Target)
	} else {
		if b.CustomDomain == "" {
			return ":443" // placeholder for the public ALB/distribution
		}
		return fmt.Sprintf("%s--%d.%s", safeFqn, port.Target, b.getProjectDomain(b.CustomDomain))
	}
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b ByocAws) GetPublicFqdn(fqn qualifiedName) string {
	safeFqn := dnsSafe(fqn)
	if b.CustomDomain == "" {
		return "" //b.fqdn
	}
	return fmt.Sprintf("%s.%s", safeFqn, b.getProjectDomain(b.CustomDomain))
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b ByocAws) GetPrivateFqdn(fqn qualifiedName) string {
	safeFqn := dnsSafe(fqn)
	return fmt.Sprintf("%s.%s", safeFqn, b.privateDomain)
}

func (b ByocAws) getProjectDomain(domain string) string {
	if strings.EqualFold(b.pulumiProject, string(b.TenantID)) {
		return domain
	}
	return dnsSafe(b.pulumiProject) + "." + domain
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func dnsSafe(fqn qualifiedName) string {
	return strings.ReplaceAll(strings.ToLower(string(fqn)), ".", "-")
}

func (b *ByocAws) TearDown(ctx context.Context) error {
	return b.Driver.TearDown(ctx)
}

func (b *ByocAws) BootstrapCommand(ctx context.Context, command string) (string, error) {
	if err := b.setUp(ctx); err != nil {
		return "", err
	}
	cdTaskArn, err := b.runCdCommand(ctx, command)
	if err != nil || cdTaskArn == nil {
		return "", annotateAwsError(err)
	}
	return ecs.GetTaskID(cdTaskArn), nil
}

func (b *ByocAws) Destroy(ctx context.Context) (string, error) {
	return b.BootstrapCommand(ctx, "down")
}

func (b *ByocAws) DeleteSecrets(ctx context.Context, secrets *defangv1.Secrets) error {
	ids := make([]string, len(secrets.Names))
	for i, name := range secrets.Names {
		ids[i] = b.getSecretID(name)
	}
	if err := b.Driver.DeleteSecrets(ctx, ids...); err != nil {
		return annotateAwsError(err)
	}
	return nil
}

func (b *ByocAws) Restart(ctx context.Context, names ...string) error {
	return errors.New("not yet implemented for BYOC; please use the AWS ECS dashboard") // FIXME: implement this for BYOC
}

func (b *ByocAws) BootstrapList(ctx context.Context) error {
	if err := b.setUp(ctx); err != nil {
		return err
	}
	cfg, err := b.Driver.LoadConfig(ctx)
	if err != nil {
		return annotateAwsError(err)
	}
	prefix := `.pulumi/stacks/` // TODO: should we filter on `projectName`?
	s3client := s3.NewFromConfig(cfg)
	out, err := s3client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &b.Driver.BucketName,
		Prefix: &prefix,
	})
	if err != nil {
		return annotateAwsError(err)
	}
	for _, obj := range out.Contents {
		// The JSON file for an empty stack is ~600 bytes; we add a margin of 100 bytes to account for the length of the stack/project names
		if obj.Key == nil || !strings.HasSuffix(*obj.Key, ".json") || obj.Size == nil || *obj.Size < 700 {
			continue
		}
		// Cut off the prefix and the .json suffix
		stack := (*obj.Key)[len(prefix) : len(*obj.Key)-5]
		fmt.Println(" - ", stack)
	}
	return nil
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

// byocServerStream is a wrapper around awsecs.EventStream that implements connect-like ServerStream
type byocServerStream struct {
	cancelTaskCh func()
	err          error
	errCh        <-chan error
	etag         string
	response     *defangv1.TailResponse
	service      string
	stream       ecs.EventStream
	taskCh       <-chan error
}

var _ client.ServerStream[defangv1.TailResponse] = (*byocServerStream)(nil)

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

func (bs *byocServerStream) Msg() *defangv1.TailResponse {
	return bs.response
}

type hasErrCh interface {
	Errs() <-chan error
}

func (bs *byocServerStream) Receive() bool {
	select {
	case e := <-bs.stream.Events(): // blocking
		events, err := ecs.GetLogEvents(e)
		if err != nil {
			bs.err = err
			return false
		}
		bs.response = &defangv1.TailResponse{}
		if len(events) == 0 {
			// The original gRPC/connect server stream would never send an empty response.
			// We could loop around the select, but returning an empty response updates the spinner.
			return true
		}
		var record logs.FirelensMessage
		parseFirelensRecords := false
		// Get the Etag/Host/Service from the first event (should be the same for all events in this batch)
		event := events[0]
		if parts := strings.Split(*event.LogStreamName, "/"); len(parts) == 3 {
			if strings.Contains(*event.LogGroupIdentifier, ":"+CdTaskPrefix) {
				// These events are from the CD task: "crun/main/taskID" stream; we should detect stdout/stderr
				bs.response.Etag = bs.etag // pass the etag filter below, but we already filtered the tail by taskID
				bs.response.Host = "pulumi"
				bs.response.Service = "cd"
			} else {
				// These events are from an awslogs service task: "tenant/service_etag/taskID" stream
				bs.response.Host = parts[2] // TODO: figure out actual hostname/IP
				parts = strings.Split(parts[1], "_")
				if len(parts) != 2 || !pkg.IsValidRandomID(parts[1]) {
					// skip, ignore sidecar logs (like route53-sidecar or fluentbit)
					return true
				}
				service, etag := parts[0], parts[1]
				bs.response.Etag = etag
				bs.response.Service = service
			}
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
		entries := make([]*defangv1.LogEntry, len(events))
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
			entries[i] = &defangv1.LogEntry{
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
