package aws

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	r53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/ptr"
	"github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/proto"
)

type ByocAws struct {
	*byoc.ByocBaseClient

	cdTasks      map[string]ecs.TaskArn
	driver       *cfn.AwsEcs
	publicNatIps []string
}

var _ client.Client = (*ByocAws)(nil)

func NewByoc(ctx context.Context, grpcClient client.GrpcClient, tenantId types.TenantID) *ByocAws {
	b := &ByocAws{
		cdTasks: make(map[string]ecs.TaskArn),
		driver:  cfn.New(byoc.CdTaskPrefix, aws.Region("")), // default region
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, grpcClient, tenantId, b)
	return b
}

func (b *ByocAws) setUp(ctx context.Context) error {
	if b.SetupDone {
		return nil
	}
	cdTaskName := byoc.CdTaskPrefix
	containers := []types.Container{
		{
			Image:     "public.ecr.aws/pulumi/pulumi-nodejs:latest",
			Name:      ecs.ContainerName,
			Cpus:      2.0,
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
			Image:     byoc.CdImage,
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
	if err := b.driver.SetUp(ctx, containers); err != nil {
		return annotateAwsError(err)
	}

	if b.ProjectDomain == "" {
		domain, err := b.GetDelegateSubdomainZone(ctx)
		if err != nil {
			term.Debug("Failed to get subdomain zone:", err)
			// return err; FIXME: ignore this error for now
		} else {
			b.ProjectDomain = b.getProjectDomain(domain.Zone)
			if b.ProjectDomain != "" {
				b.ShouldDelegateSubdomain = true
			}
		}
	}

	b.SetupDone = true
	return nil
}

func (b *ByocAws) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	etag := pkg.RandomID()
	if len(req.Services) > b.Quota.Services {
		return nil, errors.New("maximum number of services reached")
	}
	serviceInfos := []*defangv1.ServiceInfo{}
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

	if b.ShouldDelegateSubdomain {
		if _, err := b.delegateSubdomain(ctx); err != nil {
			return nil, err
		}
	}
	taskArn, err := b.runCdCommand(ctx, "up", payloadString)
	if err != nil {
		return nil, err
	}
	b.cdTasks[etag] = taskArn

	for _, si := range serviceInfos {
		if si.UseAcmeCert {
			term.Infof("To activate TLS certificate for %v, run 'defang cert gen'", si.Service.Domainname)
		}
	}

	return &defangv1.DeployResponse{
		Services: serviceInfos, // TODO: Should we use the retrieved services instead?
		Etag:     etag,
	}, nil
}

func (b *ByocAws) findZone(ctx context.Context, domain, role string) (string, error) {
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

func (b *ByocAws) delegateSubdomain(ctx context.Context) (string, error) {
	if b.ProjectDomain == "" {
		return "", errors.New("custom domain not set")
	}
	domain := b.ProjectDomain
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
	nsServers, err := aws.GetRecordsValue(ctx, zoneId, domain, r53types.RRTypeNs, r53Client)
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

func (b *ByocAws) WhoAmI(ctx context.Context) (*defangv1.WhoAmIResponse, error) {
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
	return &defangv1.WhoAmIResponse{
		Tenant:  b.TenantID,
		Region:  cfg.Region,
		Account: *identity.Account,
	}, nil
}

func (*ByocAws) GetVersions(context.Context) (*defangv1.Version, error) {
	cdVersion := byoc.CdImage[strings.LastIndex(byoc.CdImage, ":")+1:]
	return &defangv1.Version{Fabric: cdVersion}, nil
}

func (b *ByocAws) GetService(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
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

func (b *ByocAws) bucketName() string {
	return pkg.Getenv("DEFANG_CD_BUCKET", b.driver.BucketName)
}

func (b *ByocAws) environment() map[string]string {
	region := b.driver.Region // TODO: this should be the destination region, not the CD region; make customizable
	return map[string]string{
		// "AWS_REGION":               region.String(), should be set by ECS (because of CD task role)
		"DEFANG_PREFIX":              byoc.DefangPrefix,
		"DEFANG_DEBUG":               os.Getenv("DEFANG_DEBUG"), // TODO: use the global DoDebug flag
		"DEFANG_ORG":                 b.TenantID,
		"DOMAIN":                     b.ProjectDomain,
		"PRIVATE_DOMAIN":             b.PrivateDomain,
		"PROJECT":                    b.PulumiProject, // may be empty
		"PULUMI_BACKEND_URL":         fmt.Sprintf(`s3://%s?region=%s&awssdk=v2`, b.bucketName(), region),
		"PULUMI_CONFIG_PASSPHRASE":   pkg.Getenv("PULUMI_CONFIG_PASSPHRASE", "asdf"), // TODO: make customizable
		"STACK":                      b.PulumiStack,
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",
		"PULUMI_SKIP_UPDATE_CHECK":   "true",
	}
}

func (b *ByocAws) runCdCommand(ctx context.Context, cmd ...string) (ecs.TaskArn, error) {
	env := b.environment()
	if term.DoDebug() {
		debugEnv := fmt.Sprintf("AWS_REGION=%q", b.driver.Region)
		if awsProfile := os.Getenv("AWS_PROFILE"); awsProfile != "" {
			debugEnv += fmt.Sprintf(" AWS_PROFILE=%q", awsProfile)
		}
		for k, v := range env {
			debugEnv += fmt.Sprintf(" %s=%q", k, v)
		}
		term.Debug(debugEnv, "npm run dev", strings.Join(cmd, " "))
	}
	return b.driver.Run(ctx, env, cmd...)
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

// stackDir returns a stack-qualified path, like the Pulumi TS function `stackDir`
func (b *ByocAws) stackDir(name string) string {
	ensure(b.PulumiProject != "", "pulumiProject not set")
	return fmt.Sprintf("/%s/%s/%s/%s", byoc.DefangPrefix, b.PulumiProject, b.PulumiStack, name) // same as shared/common.ts
}

func (b *ByocAws) GetServices(ctx context.Context) (*defangv1.ListServicesResponse, error) {
	bucketName := b.bucketName()
	if bucketName == "" {
		if err := b.driver.FillOutputs(ctx); err != nil {
			return nil, annotateAwsError(err)
		}
		bucketName = b.bucketName()
	}

	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, annotateAwsError(err)
	}

	s3Client := s3.NewFromConfig(cfg)
	// Path to the state file, Defined at: https://github.com/DefangLabs/defang-mvp/blob/main/pulumi/cd/byoc/aws/index.ts#L89
	ensure(b.PulumiProject != "", "pulumiProject not set")
	path := fmt.Sprintf("projects/%s/%s/project.pb", b.PulumiProject, b.PulumiStack)

	term.Debug("Getting services from bucket:", bucketName, path)
	getObjectOutput, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &path,
	})
	var serviceInfos defangv1.ListServicesResponse
	if err != nil {
		if aws.IsS3NoSuchKeyError(err) {
			term.Debug("s3.GetObject:", err)
			return &serviceInfos, nil // no services yet
		}
		return nil, annotateAwsError(err)
	}
	defer getObjectOutput.Body.Close()
	pbBytes, err := io.ReadAll(getObjectOutput.Body)
	if err != nil {
		return nil, err
	}
	if err := proto.Unmarshal(pbBytes, &serviceInfos); err != nil {
		return nil, err
	}
	return &serviceInfos, nil
}

func (b *ByocAws) getSecretID(name string) string {
	return b.stackDir(name) // same as defang_service.ts
}

func (b *ByocAws) PutConfig(ctx context.Context, config *defangv1.PutValue) error {
	if !pkg.IsValidSecretName(config.Name) {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret name; must be alphanumeric or _, cannot start with a number: %q", config.Name))
	}

	isSensitive := false

	fqn := b.getSecretID(config.Name)
	term.Debugf("Putting parameter %q", fqn)

	err := b.driver.PutConfig(ctx, fqn, config.Value, isSensitive)

	return annotateAwsError(err)
}

func (b *ByocAws) GetConfig(ctx context.Context, secret *defangv1.Configs) (types.ConfigData, error) {
	paramNameToName := make(map[string]string, len(secret.Names))
	paramNames := make([]string, len(secret.Names))
	for index, name := range secret.Names {
		if !pkg.IsValidSecretName(name) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret name; must be alphanumeric or _, cannot start with a number: %q", name))
		}
		paramNames[index] = b.getSecretID(name)
		paramNameToName[paramNames[index]] = name
	}

	term.Debugf("Show parameters %q", paramNames)

	configData, err := b.driver.GetConfig(ctx, paramNames)
	if err != nil {
		term.Errorf("error getting config: %v", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to retrieve environment configs"))
	}

	results := make(types.ConfigData, 1)
	for paramName, value := range configData {
		name, ok := paramNameToName[paramName]
		if !ok {
			term.Errorf("Error finding name for parameter %q", paramName)
			continue
		}
		results[name] = value
	}

	return results, nil
}

func (b *ByocAws) ListConfig(ctx context.Context) (*defangv1.Secrets, error) {
	prefix := b.getSecretID("")
	term.Debugf("Listing parameters with prefix %q", prefix)
	awsSecrets, err := b.driver.ListSecretsByPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
	configs := make([]string, len(awsSecrets))
	for i, secret := range awsSecrets {
		configs[i] = strings.TrimPrefix(secret, prefix)
	}
	return &defangv1.Secrets{Names: configs}, nil
}

func (b *ByocAws) CreateUploadURL(ctx context.Context, req *defangv1.UploadURLRequest) (*defangv1.UploadURLResponse, error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	url, err := b.driver.CreateUploadURL(ctx, req.Digest)
	if err != nil {
		return nil, err
	}
	return &defangv1.UploadURLResponse{
		Url: url,
	}, nil
}

func (b *ByocAws) Follow(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	if err := b.setUp(ctx); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancelCause(ctx)

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
		eventStream, err = b.driver.TailTaskID(ctx, etag)
		taskArn, _ = b.driver.GetTaskArn(etag)
		term.Debug("Tailing task", etag)
		etag = "" // no need to filter by etag
	} else {
		// Tail CD, kaniko, and all services (this requires PulumiProject to be set)
		kanikoTail := ecs.LogGroupInput{LogGroupARN: b.driver.MakeARN("logs", "log-group:"+b.stackDir("builds"))} // must match logic in ecs/common.ts
		term.Debug("Tailing kaniko logs", kanikoTail.LogGroupARN)
		servicesTail := ecs.LogGroupInput{LogGroupARN: b.driver.MakeARN("logs", "log-group:"+b.stackDir("logs"))} // must match logic in ecs/common.ts
		term.Debug("Tailing services logs", servicesTail.LogGroupARN)
		ecsTail := ecs.LogGroupInput{LogGroupARN: b.driver.MakeARN("logs", "log-group:"+b.stackDir("ecs"))} // must match logic in ecs/common.ts
		term.Debug("Tailing ecs events logs", ecsTail.LogGroupARN)
		cdTail := ecs.LogGroupInput{LogGroupARN: b.driver.LogGroupARN}
		taskArn = b.cdTasks[etag]
		if taskArn != nil {
			// If we know the CD task ARN, only tail the logstream for the CD task
			cdTail.LogStreamNames = []string{ecs.GetLogStreamForTaskID(ecs.GetTaskID(taskArn))}
		}
		term.Debug("Tailing CD logs", cdTail.LogGroupARN, cdTail.LogStreamNames)
		eventStream, err = ecs.TailLogGroups(ctx, req.Since.AsTime(), cdTail, kanikoTail, servicesTail, ecsTail)
	}
	if err != nil {
		return nil, annotateAwsError(err)
	}

	if taskArn != nil {
		go func() {
			if err := ecs.WaitForTask(ctx, taskArn, 3*time.Second); err != nil {
				time.Sleep(time.Second) // make sure we got all the logs from the task before cancelling
				cancel(err)
			}
		}()
	}

	return newByocServerStream(ctx, eventStream, etag, req.GetServices()), nil
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocAws) update(ctx context.Context, service *defangv1.Service) (*defangv1.ServiceInfo, error) {
	if err := b.Quota.Validate(service); err != nil {
		return nil, err
	}

	// Check to make sure all required secrets are present in the secrets store
	missing, err := b.checkForMissingSecrets(ctx, service.Secrets)
	if err != nil {
		return nil, err
	}
	if missing != nil {
		return nil, fmt.Errorf("missing config %q", missing) // retryable CodeFailedPrecondition
	}

	ensure(b.PulumiProject != "", "pulumiProject not set")
	si := &defangv1.ServiceInfo{
		Service: service,
		Project: b.PulumiProject, // was: tenant
		Etag:    pkg.RandomID(),  // TODO: could be hash for dedup/idempotency
	}

	hasHost := false
	hasIngress := false
	fqn := service.Name
	if service.StaticFiles == nil {
		for _, port := range service.Ports {
			hasIngress = hasIngress || port.Mode == defangv1.Mode_INGRESS
			hasHost = hasHost || port.Mode == defangv1.Mode_HOST
			si.Endpoints = append(si.Endpoints, b.getEndpoint(fqn, port))
		}
	} else {
		si.PublicFqdn = b.getPublicFqdn(fqn)
		si.Endpoints = append(si.Endpoints, si.PublicFqdn)
	}
	if hasIngress {
		si.LbIps = b.PrivateLbIps // only set LB IPs if there are ingress ports
		si.PublicFqdn = b.getPublicFqdn(fqn)
	}
	if hasHost {
		si.PrivateFqdn = b.getPrivateFqdn(fqn)
	}

	if service.Domainname != "" {
		if !hasIngress && service.StaticFiles == nil {
			return nil, errors.New("domainname requires at least one ingress port") // retryable CodeFailedPrecondition
		}
		// Do a DNS lookup for Domainname and confirm it's indeed a CNAME to the service's public FQDN
		cname, _ := net.LookupCNAME(service.Domainname)
		if strings.TrimSuffix(cname, ".") != si.PublicFqdn {
			zoneId, err := b.findZone(ctx, service.Domainname, service.DnsRole)
			if err != nil {
				return nil, err
			}
			if zoneId == "" {
				si.UseAcmeCert = true
				// TODO: We should add link to documentation on how the acme cert workflow works
				// TODO: Should we make this the default behavior or require the user to set a flag?
			} else {
				si.ZoneId = zoneId
			}
		}
	}

	si.NatIps = b.publicNatIps // TODO: even internal services use NAT now
	si.Status = "UPDATE_QUEUED"
	si.State = defangv1.ServiceState_UPDATE_QUEUED
	if si.Service.Build != nil {
		si.Status = "BUILD_QUEUED" // in SaaS, this gets overwritten by the ECS events for "kaniko"
		si.State = defangv1.ServiceState_BUILD_QUEUED
	}
	return si, nil
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocAws) checkForMissingSecrets(ctx context.Context, secrets []*defangv1.Secret) (*defangv1.Secret, error) {
	if len(secrets) == 0 {
		return nil, nil // no secrets to check
	}
	prefix := b.getSecretID("")
	sorted, err := b.driver.ListSecretsByPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
	for _, secret := range secrets {
		fqn := b.getSecretID(secret.Source)
		if !searchSecret(sorted, fqn) {
			return secret, nil // secret not found
		}
	}
	return nil, nil // all secrets found
}

// This function was copied from Fabric controller
func searchSecret(sorted []qualifiedName, fqn qualifiedName) bool {
	i := sort.Search(len(sorted), func(i int) bool {
		return sorted[i] >= fqn
	})
	return i < len(sorted) && sorted[i] == fqn
}

type qualifiedName = string // legacy

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocAws) getEndpoint(fqn qualifiedName, port *defangv1.Port) string {
	if port.Mode == defangv1.Mode_HOST {
		privateFqdn := b.getPrivateFqdn(fqn)
		return fmt.Sprintf("%s:%d", privateFqdn, port.Target)
	}
	if b.ProjectDomain == "" {
		return ":443" // placeholder for the public ALB/distribution
	}
	safeFqn := byoc.DnsSafeLabel(fqn)
	return fmt.Sprintf("%s--%d.%s", safeFqn, port.Target, b.ProjectDomain)

}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocAws) getPublicFqdn(fqn qualifiedName) string {
	if b.ProjectDomain == "" {
		return "" //b.fqdn
	}
	safeFqn := byoc.DnsSafeLabel(fqn)
	return fmt.Sprintf("%s.%s", safeFqn, b.ProjectDomain)
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocAws) getPrivateFqdn(fqn qualifiedName) string {
	safeFqn := byoc.DnsSafeLabel(fqn)
	return fmt.Sprintf("%s.%s", safeFqn, b.PrivateDomain) // TODO: consider merging this with ServiceDNS
}

func (b *ByocAws) getProjectDomain(zone string) string {
	if b.PulumiProject == "" {
		return "" // no project name => no custom domain
	}
	projectLabel := byoc.DnsSafeLabel(b.PulumiProject)
	if projectLabel == byoc.DnsSafeLabel(b.TenantID) {
		return byoc.DnsSafe(zone) // the zone will already have the tenant ID
	}
	return projectLabel + "." + byoc.DnsSafe(zone)
}

func (b *ByocAws) TearDown(ctx context.Context) error {
	return b.driver.TearDown(ctx)
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

func (b *ByocAws) DeleteConfig(ctx context.Context, secrets *defangv1.Secrets) error {
	ids := make([]string, len(secrets.Names))
	for i, name := range secrets.Names {
		ids[i] = b.getSecretID(name)
	}
	term.Debug("Deleting parameters", ids)
	if err := b.driver.DeleteSecrets(ctx, ids...); err != nil {
		return annotateAwsError(err)
	}
	return nil
}

func (b *ByocAws) Restart(ctx context.Context, names ...string) (types.ETag, error) {
	return "", client.ErrNotImplemented("not yet implemented for BYOC; please use the AWS ECS dashboard") // FIXME: implement this for BYOC
}

func (b *ByocAws) BootstrapList(ctx context.Context) ([]string, error) {
	bucketName := b.bucketName()
	if bucketName == "" {
		if err := b.driver.FillOutputs(ctx); err != nil {
			return nil, annotateAwsError(err)
		}
		bucketName = b.bucketName()
	}

	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, annotateAwsError(err)
	}

	s3client := s3.NewFromConfig(cfg)
	prefix := `.pulumi/stacks/` // TODO: should we filter on `projectName`?

	term.Debug("Listing stacks in bucket:", bucketName)
	out, err := s3client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &bucketName,
		Prefix: &prefix,
	})
	if err != nil {
		return nil, annotateAwsError(err)
	}
	var stacks []string
	for _, obj := range out.Contents {
		// The JSON file for an empty stack is ~600 bytes; we add a margin of 100 bytes to account for the length of the stack/project names
		if obj.Key == nil || !strings.HasSuffix(*obj.Key, ".json") || obj.Size == nil || *obj.Size < 700 {
			continue
		}
		// Cut off the prefix and the .json suffix
		stack := (*obj.Key)[len(prefix) : len(*obj.Key)-5]
		stacks = append(stacks, stack)
	}
	return stacks, nil
}

// annotateAwsError translates the AWS error to an error code the CLI client understands
func annotateAwsError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "get credentials:") {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	if aws.IsS3NoSuchKeyError(err) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if aws.IsParameterNotFoundError(err) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	return err
}

func ensure(cond bool, msg string) {
	if !cond {
		panic(msg)
	}
}

func (b *ByocAws) Subscribe(context.Context, *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	return nil, client.ErrNotImplemented("not yet implemented for BYOC; please use the AWS ECS dashboard") // FIXME: implement this for BYOC
}
