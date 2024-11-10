package aws

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
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
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
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"google.golang.org/protobuf/proto"
)

const (
	CdImageRepo = "public.ecr.aws/defang-io/cd"
)

var (
	PulumiVersion = pkg.Getenv("DEFANG_PULUMI_VERSION", "3.136.1")
)

type ByocAws struct {
	*byoc.ByocBaseClient

	driver *cfn.AwsEcs // TODO: ecs is stateful, contains the output of the cd cfn stack after setUpCD

	ecsEventHandlers []ECSEventHandler
	handlersLock     sync.RWMutex
	lastCdEtag       types.ETag
	lastCdStart      time.Time
	lastCdTaskArn    ecs.TaskArn
}

var _ client.Provider = (*ByocAws)(nil)

func NewByocProvider(ctx context.Context, tenantId types.TenantID) *ByocAws {
	b := &ByocAws{
		driver: cfn.New(byoc.CdTaskPrefix, aws.Region("")), // default region
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, tenantId, b)
	return b
}

func (b *ByocAws) setUpCD(ctx context.Context, projectName string) (string, error) {
	if b.SetupDone {
		return "", nil
	}

	// note: the CD image is tagged with the major release number, use that for setup
	projectCdImageTag, err := b.getCdImageTag(ctx, projectName)
	if err != nil {
		return "", err
	}

	cdTaskName := byoc.CdTaskPrefix
	containers := []types.Container{
		{
			Image:     "public.ecr.aws/pulumi/pulumi-nodejs:" + PulumiVersion,
			Name:      ecs.CdContainerName,
			Cpus:      2.0,
			Memory:    2048_000_000, // 2G
			Essential: ptr.Bool(true),
			VolumesFrom: []string{
				cdTaskName,
			},
			WorkDir:    "/app",
			DependsOn:  map[string]types.ContainerCondition{cdTaskName: "START"},
			EntryPoint: []string{"node", "lib/index.js"},
		},
		{
			Image:     byoc.GetCdImage(CdImageRepo, projectCdImageTag),
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
		return "", byoc.AnnotateAwsError(err)
	}

	b.SetupDone = true
	return projectCdImageTag, nil
}

func (b *ByocAws) getCdImageTag(ctx context.Context, projectName string) (string, error) {
	// see if we have a previous deployment; use the same cd image tag
	projUpdate, err := b.getProjectUpdate(ctx, projectName)
	if err != nil {
		return "", err
	}

	// older deployments may not have the cd_version field set,
	// these would have been deployed with public-beta
	if projUpdate != nil && projUpdate.CdVersion == "" {
		projUpdate.CdVersion = byoc.CdDefaultImageTag
	}

	// send project update with the current deploy's cd image tag,
	// most current version if new deployment
	imagePath := byoc.GetCdImage(CdImageRepo, byoc.CdLatestImageTag)
	deploymentCdImageTag := byoc.ExtractImageTag(imagePath)
	if (projUpdate != nil) && (len(projUpdate.Services) > 0) && (projUpdate.CdVersion != "") {
		deploymentCdImageTag = projUpdate.CdVersion
	}

	// possible values are [public-beta, 1, 2,...]
	return deploymentCdImageTag, nil
}

func (b *ByocAws) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "up")
}

func (b *ByocAws) Preview(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "preview")
}

func (b *ByocAws) deploy(ctx context.Context, req *defangv1.DeployRequest, cmd string) (*defangv1.DeployResponse, error) {
	// If multiple Compose files were provided, req.Compose is the merged representation of all the files
	project, err := compose.LoadFromContent(ctx, req.Compose, "")
	if err != nil {
		return nil, err
	}

	cdImageTag, err := b.setUpCD(ctx, project.Name)
	if err != nil {
		return nil, err
	}

	etag := pkg.RandomID()
	if len(project.Services) > b.Quota.Services {
		return nil, errors.New("maximum number of services reached")
	}

	serviceInfos := []*defangv1.ServiceInfo{}
	for _, service := range project.Services {
		serviceInfo, err := b.update(ctx, project.Name, req.DelegateDomain, service)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", service.Name, err)
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

	data, err := proto.Marshal(&defangv1.ProjectUpdate{
		CdVersion: cdImageTag,
		Compose:   req.Compose,
		Services:  serviceInfos,
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
		payloadUrl, err := b.driver.CreateUploadURL(ctx, etag)
		if err != nil {
			return nil, err
		}

		// Do an HTTP PUT to the generated URL
		resp, err := http.Put(ctx, payloadUrl, "application/protobuf", bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("unexpected status code during upload: %s", resp.Status)
		}
		payloadString = http.RemoveQueryParam(payloadUrl)
	}

	cdCommand := cdCmd{
		mode:            req.Mode,
		project:         project.Name,
		delegateDomain:  req.DelegateDomain,
		delegationSetId: req.DelegationSetId,
		cmd:             []string{cmd, payloadString},
	}
	taskArn, err := b.runCdCommand(ctx, cdCommand)
	if err != nil {
		return nil, err
	}
	b.lastCdEtag = etag
	b.lastCdStart = time.Now()
	b.lastCdTaskArn = taskArn

	for _, si := range serviceInfos {
		if si.UseAcmeCert {
			term.Infof("To activate TLS certificate for %v, run 'defang cert gen'", si.Domainname)
		}
	}

	return &defangv1.DeployResponse{
		Services: serviceInfos, // TODO: Should we use the retrieved services instead?
		Etag:     etag,
	}, nil
}

func (b *ByocAws) findZone(ctx context.Context, domain, roleARN string) (string, error) {
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return "", byoc.AnnotateAwsError(err)
	}

	if roleARN != "" {
		stsClient := sts.NewFromConfig(cfg)
		creds := stscreds.NewAssumeRoleProvider(stsClient, roleARN)
		cfg.Credentials = awssdk.NewCredentialsCache(creds)
	}

	r53Client := route53.NewFromConfig(cfg)

	domain = strings.TrimSuffix(domain, ".")
	domain = strings.ToLower(domain)
	for {
		zone, err := aws.GetHostedZoneByName(ctx, domain, r53Client)
		if errors.Is(err, aws.ErrZoneNotFound) {
			if strings.Count(domain, ".") <= 1 {
				return "", nil
			}
			domain = domain[strings.Index(domain, ".")+1:]
			continue
		} else if err != nil {
			return "", err
		}
		return *zone.Id, nil
	}
}

func (b *ByocAws) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	projectDomain := b.GetProjectDomain(req.Project, req.DelegateDomain)

	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, byoc.AnnotateAwsError(err)
	}
	r53Client := route53.NewFromConfig(cfg)

	// There's four cases to consider:
	//  1. The subdomain zone does not exist: we get NS records from the delegation set and let CD/Pulumi create the hosted zone
	//  2. The subdomain zone exists:
	//    a. The zone was created by the older CLI: we need to get the NS records from the existing zone
	//    b. The zone was created by the new CD/Pulumi: we get the NS records from the delegation set and let CD/Pulumi create the hosted zone
	//    c. The zone was created another way: the deployment will likely fail with a "zone already exists" error

	var nsServers []string
	zone, err := aws.GetHostedZoneByName(ctx, projectDomain, r53Client)
	if err != nil {
		if !errors.Is(err, aws.ErrZoneNotFound) {
			return nil, byoc.AnnotateAwsError(err) // TODO: we should not fail deployment if this fails
		}
		term.Debugf("Zone %q not found, delegation set will be created", projectDomain)
		// Case 1: The zone doesn't exist: we'll create a delegation set and let CD/Pulumi create the hosted zone
	} else {
		// Case 2: Get the NS records for the existing subdomain zone
		nsServers, err = aws.ListResourceRecords(ctx, *zone.Id, projectDomain, r53types.RRTypeNs, r53Client)
		if err != nil {
			return nil, byoc.AnnotateAwsError(err) // TODO: we should not fail deployment if this fails
		}
		term.Debugf("Zone %q found, NS records: %v", projectDomain, nsServers)
	}

	var resp client.PrepareDomainDelegationResponse
	if zone == nil || zone.Config.Comment == nil || *zone.Config.Comment != aws.CreateHostedZoneComment {
		// Case 2b or 2c: The zone does not exist, or was not created by an older version of this CLI.
		// Get the NS records for the delegation set (using the existing zone) and let Pulumi create the hosted zone for us
		var zoneId *string
		if zone != nil {
			zoneId = zone.Id
		}
		delegationSet, err := aws.CreateDelegationSet(ctx, zoneId, r53Client)
		var delegationSetAlreadyCreated *r53types.DelegationSetAlreadyCreated
		var delegationSetAlreadyReusable *r53types.DelegationSetAlreadyReusable
		if errors.As(err, &delegationSetAlreadyCreated) || errors.As(err, &delegationSetAlreadyReusable) {
			term.Debug("Route53 delegation set already created:", err)
			delegationSet, err = aws.GetDelegationSet(ctx, r53Client)
		}
		if err != nil {
			return nil, byoc.AnnotateAwsError(err)
		}
		if len(delegationSet.NameServers) == 0 {
			return nil, errors.New("no NS records found for the delegation set") // should not happen
		}
		term.Debug("Route53 delegation set ID:", *delegationSet.Id)
		resp.DelegationSetId = strings.TrimPrefix(*delegationSet.Id, "/delegationset/")

		// Ensure the NS records match the ones from the delegation set if the zone already exists
		if zoneId != nil {
			sort.Strings(nsServers)
			sort.Strings(delegationSet.NameServers)
			if !slices.Equal(delegationSet.NameServers, nsServers) {
				track.Evt("Compose-Up delegateSubdomain diff", track.P("fromDS", delegationSet.NameServers), track.P("fromZone", nsServers))
				term.Debugf("NS records for the existing subdomain zone do not match the delegation set: %v <> %v", delegationSet.NameServers, nsServers)
			}
		}

		nsServers = delegationSet.NameServers
	} else {
		// Case 2a: The zone was created by the older CLI, we'll use the existing NS records; track how many times this happens
		track.Evt("Compose-Up delegateSubdomain old", track.P("domain", projectDomain))
	}
	resp.NameServers = nsServers

	return &resp, nil
}

func (b *ByocAws) AccountInfo(ctx context.Context) (client.AccountInfo, error) {
	// Use STS to get the account ID
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, byoc.AnnotateAwsError(err)
	}
	identity, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, byoc.AnnotateAwsError(err)
	}
	return AWSAccountInfo{
		region:    cfg.Region,
		accountID: *identity.Account,
		arn:       *identity.Arn,
	}, nil
}

type AWSAccountInfo struct {
	accountID string
	region    string
	arn       string
}

func (i AWSAccountInfo) AccountID() string {
	return i.accountID
}

func (i AWSAccountInfo) Region() string {
	return i.region
}

func (i AWSAccountInfo) Details() string {
	return i.arn
}

func (b *ByocAws) GetService(ctx context.Context, s *defangv1.ServiceID) (*defangv1.ServiceInfo, error) {
	all, err := b.GetServices(ctx, &defangv1.GetServicesRequest{Project: s.Project})
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

func (b *ByocAws) environment(projectName string) map[string]string {
	region := b.driver.Region // TODO: this should be the destination region, not the CD region; make customizable
	return map[string]string{
		// "AWS_REGION":               region.String(), should be set by ECS (because of CD task role)
		"DEFANG_DEBUG":               os.Getenv("DEFANG_DEBUG"), // TODO: use the global DoDebug flag
		"DEFANG_ORG":                 b.TenantID,
		"DEFANG_PREFIX":              byoc.DefangPrefix,
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",
		"PRIVATE_DOMAIN":             byoc.GetPrivateDomain(projectName),
		"PROJECT":                    projectName, // may be empty
		"PULUMI_BACKEND_URL":         fmt.Sprintf(`s3://%s?region=%s&awssdk=v2`, b.bucketName(), region),
		"PULUMI_CONFIG_PASSPHRASE":   pkg.Getenv("PULUMI_CONFIG_PASSPHRASE", "asdf"), // TODO: make customizable
		"PULUMI_SKIP_UPDATE_CHECK":   "true",
		"STACK":                      b.PulumiStack,
	}
}

type cdCmd struct {
	mode            defangv1.DeploymentMode
	project         string
	delegateDomain  string
	delegationSetId string
	cmd             []string
}

func (b *ByocAws) runCdCommand(ctx context.Context, cmd cdCmd) (ecs.TaskArn, error) {
	// Setup the deployment environment
	env := b.environment(cmd.project)
	if cmd.delegationSetId != "" {
		env["DELEGATION_SET_ID"] = cmd.delegationSetId
	}
	if cmd.delegateDomain != "" {
		env["DOMAIN"] = b.GetProjectDomain(cmd.project, cmd.delegateDomain)
	} else {
		env["DOMAIN"] = "dummy.domain"
	}
	env["DEFANG_MODE"] = strings.ToLower(cmd.mode.String())

	if term.DoDebug() {
		// Convert the environment to a human-readable array of KEY=VALUE strings for debugging
		debugEnv := []string{"AWS_REGION=" + b.driver.Region.String()}
		if awsProfile := os.Getenv("AWS_PROFILE"); awsProfile != "" {
			debugEnv = append(debugEnv, "AWS_PROFILE="+awsProfile)
		}
		for k, v := range env {
			debugEnv = append(debugEnv, k+"="+v)
		}
		if err := byoc.DebugPulumi(ctx, debugEnv, cmd.cmd...); err != nil {
			return nil, err
		}
	}
	return b.driver.Run(ctx, env, cmd.cmd...)
}

func (b *ByocAws) Delete(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	if len(req.Names) > 0 {
		return nil, client.ErrNotImplemented("per-service deletion is not supported for BYOC")
	}
	if _, err := b.setUpCD(ctx, req.Project); err != nil {
		return nil, err
	}
	// FIXME: this should only delete the services that are specified in the request, not all
	cmd := cdCmd{
		mode:           defangv1.DeploymentMode_UNSPECIFIED_MODE,
		project:        req.Project,
		delegateDomain: req.DelegateDomain,
		cmd:            []string{"up", ""}, // 2nd empty string is a empty payload
	}
	taskArn, err := b.runCdCommand(ctx, cmd)
	if err != nil {
		return nil, byoc.AnnotateAwsError(err)
	}
	etag := ecs.GetTaskID(taskArn) // TODO: this is the CD task ID, not the etag
	b.lastCdEtag = etag
	b.lastCdStart = time.Now()
	b.lastCdTaskArn = taskArn
	return &defangv1.DeleteResponse{Etag: etag}, nil
}

// stackDir returns a stack-qualified path, like the Pulumi TS function `stackDir`
func (b *ByocAws) stackDir(projectName, name string) string {
	ensure(projectName != "", "ProjectName not set")
	return fmt.Sprintf("/%s/%s/%s/%s", byoc.DefangPrefix, projectName, b.PulumiStack, name) // same as shared/common.ts
}

func (b *ByocAws) getProjectUpdate(ctx context.Context, projectName string) (*defangv1.ProjectUpdate, error) {
	if projectName == "" {
		return nil, nil
	}
	bucketName := b.bucketName()
	if bucketName == "" {
		if err := b.driver.FillOutputs(ctx); err != nil {
			// FillOutputs might fail if the stack is not created yet; return empty update in that case
			var cfnErr *cfn.ErrStackNotFoundException
			if errors.As(err, &cfnErr) {
				return nil, nil // no services yet
			}
			return nil, byoc.AnnotateAwsError(err)
		}
		bucketName = b.bucketName()
	}

	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, byoc.AnnotateAwsError(err)
	}

	s3Client := s3.NewFromConfig(cfg)
	// Path to the state file, Defined at: https://github.com/DefangLabs/defang-mvp/blob/main/pulumi/cd/byoc/aws/index.ts#L89
	ensure(projectName != "", "ProjectName not set")
	path := fmt.Sprintf("projects/%s/%s/project.pb", projectName, b.PulumiStack)

	term.Debug("Getting services from bucket:", bucketName, path)
	getObjectOutput, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucketName,
		Key:    &path,
	})

	if err != nil {
		if aws.IsS3NoSuchKeyError(err) {
			term.Debug("s3.GetObject:", err)
			return nil, nil // no services yet
		}
		return nil, byoc.AnnotateAwsError(err)
	}
	defer getObjectOutput.Body.Close()
	pbBytes, err := io.ReadAll(getObjectOutput.Body)
	if err != nil {
		return nil, err
	}

	projUpdate := defangv1.ProjectUpdate{}
	if err := proto.Unmarshal(pbBytes, &projUpdate); err != nil {
		return nil, err
	}

	return &projUpdate, nil
}

func (b *ByocAws) GetServices(ctx context.Context, req *defangv1.GetServicesRequest) (*defangv1.ListServicesResponse, error) {
	projUpdate, err := b.getProjectUpdate(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	listServiceResp := defangv1.ListServicesResponse{}
	if projUpdate != nil {
		listServiceResp.Services = projUpdate.Services
		listServiceResp.Project = projUpdate.Project
	}

	return &listServiceResp, nil
}

func (b *ByocAws) getSecretID(projectName, name string) string {
	return b.stackDir(projectName, name) // same as defang_service.ts
}

func (b *ByocAws) PutConfig(ctx context.Context, secret *defangv1.PutConfigRequest) error {
	if !pkg.IsValidSecretName(secret.Name) {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret name; must be alphanumeric or _, cannot start with a number: %q", secret.Name))
	}
	fqn := b.getSecretID(secret.Project, secret.Name)
	term.Debugf("Putting parameter %q", fqn)
	err := b.driver.PutSecret(ctx, fqn, secret.Value)
	return byoc.AnnotateAwsError(err)
}

func (b *ByocAws) ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error) {
	prefix := b.getSecretID(req.Project, "")
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
	if _, err := b.setUpCD(ctx, req.Project); err != nil {
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

func (b *ByocAws) Query(ctx context.Context, req *defangv1.DebugRequest) error {
	// The LogStreamNamePrefix filter can only be used with one service name
	var service string
	if len(req.Services) == 1 {
		service = req.Services[0]
	}

	since := b.lastCdStart // TODO: get start time from req.Etag
	if since.IsZero() {
		since = time.Now().Add(-time.Hour)
	}

	// Gather logs from the CD task, kaniko, ECS events, and all services
	sb := strings.Builder{}
	for _, lgi := range b.getLogGroupInputs(req.Etag, req.Project, service) {
		parseECSEventRecords := strings.HasSuffix(lgi.LogGroupARN, "/ecs")
		if err := ecs.Query(ctx, lgi, since, time.Now(), func(logEvents []ecs.LogEvent) {
			for _, event := range logEvents {
				msg := term.StripAnsi(*event.Message)
				if parseECSEventRecords {
					if event, err := ecs.ParseECSEvent([]byte(msg)); err == nil {
						// TODO: once we know the AWS deploymentId from TaskStateChangeEvent detail.startedBy, we can do a 2nd query to filter by deploymentId
						if event.Etag() != "" && req.Etag != "" && event.Etag() != req.Etag {
							continue
						}
						if event.Service() != "" && len(req.Services) > 0 && !slices.Contains(req.Services, event.Service()) {
							continue
						}
						// This matches the status messages in the Defang Playground Loki logs
						sb.WriteString("status=")
						sb.WriteString(event.Status())
						sb.WriteByte('\n')
						continue
					}
				}
				sb.WriteString(msg)
				sb.WriteByte('\n')
			}
		}); err != nil {
			term.Warn("CloudWatch query failed:", byoc.AnnotateAwsError(err))
			// continue reading other log groups
		}
	}

	req.Logs = sb.String()
	return nil
}

func (b *ByocAws) Follow(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
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
	stopWhenCDTaskDone := false
	if etag != "" && !pkg.IsValidRandomID(etag) { // Assume invalid "etag" is a task ID
		eventStream, err = b.driver.TailTaskID(ctx, etag)
		taskArn, _ = b.driver.GetTaskArn(etag)
		term.Debugf("Tailing task %s", *taskArn)
		etag = "" // no need to filter by etag
		stopWhenCDTaskDone = true
	} else {
		var service string
		if len(req.Services) == 1 {
			service = req.Services[0]
		}
		eventStream, err = ecs.TailLogGroups(ctx, req.Since.AsTime(), b.getLogGroupInputs(etag, req.Project, service)...)
		taskArn = b.lastCdTaskArn
	}
	if err != nil {
		return nil, byoc.AnnotateAwsError(err)
	}
	if taskArn != nil {
		var cancel context.CancelCauseFunc
		ctx, cancel = context.WithCancelCause(ctx)
		go func() {
			if err := ecs.WaitForTask(ctx, taskArn, 3*time.Second); err != nil {
				if stopWhenCDTaskDone || errors.As(err, &ecs.TaskFailure{}) {
					time.Sleep(time.Second) // make sure we got all the logs from the task before cancelling
					cancel(err)
				}
			}
		}()
	}

	return newByocServerStream(ctx, eventStream, etag, req.GetServices(), b), nil
}

func (b *ByocAws) makeLogGroupARN(name string) string {
	return b.driver.MakeARN("logs", "log-group:"+name)
}

func (b *ByocAws) getLogGroupInputs(etag types.ETag, projectName, service string) []ecs.LogGroupInput {
	var serviceLogsPrefix string
	if service != "" {
		serviceLogsPrefix = service + "/" + service + "_" + etag
	}
	// Tail CD, kaniko, and all services (this requires ProjectName to be set)
	kanikoTail := ecs.LogGroupInput{LogGroupARN: b.makeLogGroupARN(b.stackDir(projectName, "builds"))} // must match logic in ecs/common.ts; TODO: filter by etag/service
	term.Debug("Query kaniko logs", kanikoTail.LogGroupARN)
	servicesTail := ecs.LogGroupInput{LogGroupARN: b.makeLogGroupARN(b.stackDir(projectName, "logs")), LogStreamNamePrefix: serviceLogsPrefix} // must match logic in ecs/common.ts
	term.Debug("Query services logs", servicesTail.LogGroupARN, serviceLogsPrefix)
	ecsTail := ecs.LogGroupInput{LogGroupARN: b.makeLogGroupARN(b.stackDir(projectName, "ecs"))} // must match logic in ecs/common.ts; TODO: filter by etag/service/deploymentId
	term.Debug("Query ecs events logs", ecsTail.LogGroupARN)
	cdTail := ecs.LogGroupInput{LogGroupARN: b.driver.LogGroupARN} // TODO: filter by etag
	// If we know the CD task ARN, only tail the logstream for that CD task
	if b.lastCdTaskArn != nil && b.lastCdEtag == etag {
		cdTail.LogStreamNames = []string{ecs.GetCDLogStreamForTaskID(ecs.GetTaskID(b.lastCdTaskArn))}
	}
	term.Debug("Query CD logs", cdTail.LogGroupARN, cdTail.LogStreamNames)
	return []ecs.LogGroupInput{cdTail, kanikoTail, servicesTail, ecsTail} // more or less in chronological order
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocAws) update(ctx context.Context, projectName, delegateDomain string, service composeTypes.ServiceConfig) (*defangv1.ServiceInfo, error) {
	if err := compose.ValidateService(&service); err != nil {
		return nil, err
	}

	ensure(projectName != "", "ProjectName not set")
	si := &defangv1.ServiceInfo{
		Etag:    pkg.RandomID(), // TODO: could be hash for dedup/idempotency
		Project: projectName,    // was: tenant
		Service: &defangv1.Service{Name: service.Name},
	}

	hasHost := false
	hasIngress := false
	fqn := service.Name
	if _, ok := service.Extensions["x-defang-static-files"]; !ok {
		for _, port := range service.Ports {
			hasIngress = hasIngress || port.Mode == compose.Mode_INGRESS
			hasHost = hasHost || port.Mode == compose.Mode_HOST
			si.Endpoints = append(si.Endpoints, b.getEndpoint(projectName, delegateDomain, fqn, &port))
			mode := defangv1.Mode_INGRESS
			if port.Mode == compose.Mode_HOST {
				mode = defangv1.Mode_HOST
			}
			si.Service.Ports = append(si.Service.Ports, &defangv1.Port{
				Target: port.Target,
				Mode:   mode,
			})
		}
	} else {
		si.PublicFqdn = b.getPublicFqdn(projectName, delegateDomain, fqn)
		si.Endpoints = append(si.Endpoints, si.PublicFqdn)
	}
	if hasIngress {
		// si.LbIps = b.PrivateLbIps // only set LB IPs if there are ingress ports // FIXME: double check this is not being used at all
		si.PublicFqdn = b.getPublicFqdn(projectName, delegateDomain, fqn)
	}
	if hasHost {
		si.PrivateFqdn = b.getPrivateFqdn(projectName, fqn)
	}

	if service.DomainName != "" {
		if !hasIngress && service.Extensions["x-defang-static-files"] == nil {
			return nil, errors.New("domainname requires at least one ingress port") // retryable CodeFailedPrecondition
		}
		// Do a DNS lookup for DomainName and confirm it's indeed a CNAME to the service's public FQDN
		cname, _ := net.LookupCNAME(service.DomainName)
		if strings.TrimSuffix(cname, ".") != si.PublicFqdn {
			dnsRole, _ := service.Extensions["x-defang-dns-role"].(string)
			zoneId, err := b.findZone(ctx, service.DomainName, dnsRole)
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

	si.Status = "UPDATE_QUEUED"
	si.State = defangv1.ServiceState_UPDATE_QUEUED
	if service.Build != nil {
		si.Status = "BUILD_QUEUED" // in SaaS, this gets overwritten by the ECS events for "kaniko"
		si.State = defangv1.ServiceState_BUILD_QUEUED
	}
	return si, nil
}

type qualifiedName = string // legacy

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocAws) getEndpoint(fqn qualifiedName, projectName, delegateDomain string, port *composeTypes.ServicePortConfig) string {
	if port.Mode == compose.Mode_HOST {
		privateFqdn := b.getPrivateFqdn(projectName, fqn)
		return fmt.Sprintf("%s:%d", privateFqdn, port.Target)
	}
	projectDomain := b.GetProjectDomain(projectName, delegateDomain)
	if projectDomain == "" {
		return ":443" // placeholder for the public ALB/distribution
	}
	safeFqn := byoc.DnsSafeLabel(fqn)
	return fmt.Sprintf("%s--%d.%s", safeFqn, port.Target, projectDomain)
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocAws) getPublicFqdn(projectName, delegateDomain, fqn qualifiedName) string {
	if projectName == "" {
		return "" //b.fqdn
	}
	safeFqn := byoc.DnsSafeLabel(fqn)
	return fmt.Sprintf("%s.%s", safeFqn, b.GetProjectDomain(projectName, delegateDomain))
}

// This function was copied from Fabric controller and slightly modified to work with BYOC
func (b *ByocAws) getPrivateFqdn(projectName string, fqn qualifiedName) string {
	safeFqn := byoc.DnsSafeLabel(fqn)
	return fmt.Sprintf("%s.%s", safeFqn, byoc.GetPrivateDomain(projectName)) // TODO: consider merging this with ServiceDNS
}

func (b *ByocAws) TearDown(ctx context.Context) error {
	return b.driver.TearDown(ctx)
}

func (b *ByocAws) BootstrapCommand(ctx context.Context, req client.BootstrapCommandRequest) (string, error) {
	if _, err := b.setUpCD(ctx, req.Project); err != nil {
		return "", err
	}
	cmd := cdCmd{
		mode:    defangv1.DeploymentMode_UNSPECIFIED_MODE,
		project: req.Project,
		cmd:     []string{req.Command},
	}
	cdTaskArn, err := b.runCdCommand(ctx, cmd) // TODO: make domain optional for defang cd
	if err != nil || cdTaskArn == nil {
		return "", byoc.AnnotateAwsError(err)
	}
	return ecs.GetTaskID(cdTaskArn), nil
}

func (b *ByocAws) Destroy(ctx context.Context, req *defangv1.DestroyRequest) (string, error) {
	return b.BootstrapCommand(ctx, client.BootstrapCommandRequest{Project: req.Project, Command: "down"})
}

func (b *ByocAws) DeleteConfig(ctx context.Context, secrets *defangv1.Secrets) error {
	ids := make([]string, len(secrets.Names))
	for i, name := range secrets.Names {
		ids[i] = b.getSecretID(secrets.Project, name)
	}
	term.Debug("Deleting parameters", ids)
	if err := b.driver.DeleteSecrets(ctx, ids...); err != nil {
		return byoc.AnnotateAwsError(err)
	}
	return nil
}

func (b *ByocAws) BootstrapList(ctx context.Context) ([]string, error) {
	bucketName := b.bucketName()
	if bucketName == "" {
		if err := b.driver.FillOutputs(ctx); err != nil {
			return nil, byoc.AnnotateAwsError(err)
		}
		bucketName = b.bucketName()
	}

	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, byoc.AnnotateAwsError(err)
	}

	s3client := s3.NewFromConfig(cfg)
	prefix := `.pulumi/stacks/` // TODO: should we filter on `projectName`?

	term.Debug("Listing stacks in bucket:", bucketName)
	out, err := s3client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &bucketName,
		Prefix: &prefix,
	})
	if err != nil {
		return nil, byoc.AnnotateAwsError(err)
	}
	var stacks []string
	for _, obj := range out.Contents {
		// The JSON file for an empty stack is ~600 bytes; we add a margin of 100 bytes to account for the length of the stack/project names
		if obj.Key == nil || !strings.HasSuffix(*obj.Key, ".json") || obj.Size == nil || *obj.Size < 700 {
			continue
		}
		// Cut off the prefix and the .json suffix
		stack := (*obj.Key)[len(prefix) : len(*obj.Key)-5]
		// Check the contents of the JSON file, because the size is not a reliable indicator of a valid stack
		objOutput, err := s3client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: &bucketName,
			Key:    obj.Key,
		})
		if err != nil {
			term.Debugf("Failed to get Pulumi state object %q: %v", *obj.Key, err)
		} else {
			defer objOutput.Body.Close()
			var state struct {
				Version    int `json:"version"`
				Checkpoint struct {
					// Stack  string `json:"stack"` TODO: could use this instead of deriving the stack name from the key
					Latest struct {
						Resources         []struct{} `json:"resources,omitempty"`
						PendingOperations []struct {
							Resource struct {
								Urn string `json:"urn"`
							}
						} `json:"pending_operations,omitempty"`
					}
				}
			}
			if err := json.NewDecoder(objOutput.Body).Decode(&state); err != nil {
				term.Debugf("Failed to decode Pulumi state %q: %v", *obj.Key, err)
			} else if state.Version != 3 {
				term.Debug("Skipping Pulumi state with version", state.Version)
			} else if len(state.Checkpoint.Latest.PendingOperations) > 0 {
				for _, op := range state.Checkpoint.Latest.PendingOperations {
					parts := strings.Split(op.Resource.Urn, "::") // prefix::project::type::resource => urn:provider:stack::project::plugin:file:class::name
					stack += fmt.Sprintf(" (pending %q)", parts[3])
				}
			} else if len(state.Checkpoint.Latest.Resources) == 0 {
				continue // skip: no resources and no pending operations
			}
		}

		stacks = append(stacks, stack)
	}
	return stacks, nil
}

func ensure(cond bool, msg string) {
	if !cond {
		panic(msg)
	}
}

type ECSEventHandler interface {
	HandleECSEvent(evt ecs.Event)
}

func (b *ByocAws) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	s := &byocSubscribeServerStream{
		services: req.Services,
		etag:     req.Etag,
		ctx:      ctx,

		ch: make(chan *defangv1.SubscribeResponse),
	}
	b.AddEcsEventHandler(s)
	return s, nil
}

func (b *ByocAws) HandleECSEvent(evt ecs.Event) {
	b.handlersLock.RLock()
	defer b.handlersLock.RUnlock()
	for _, handler := range b.ecsEventHandlers {
		handler.HandleECSEvent(evt)
	}
}

func (b *ByocAws) AddEcsEventHandler(handler ECSEventHandler) {
	b.handlersLock.Lock()
	defer b.handlersLock.Unlock()
	b.ecsEventHandlers = append(b.ecsEventHandlers, handler)
}
