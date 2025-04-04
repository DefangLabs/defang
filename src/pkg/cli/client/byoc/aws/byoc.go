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
	"slices"
	"strconv"
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
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/ptr"
	"github.com/bufbuild/connect-go"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"google.golang.org/protobuf/proto"
)

var (
	PulumiVersion = pkg.Getenv("DEFANG_PULUMI_VERSION", "3.148.0")
)

type StsProviderAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
	AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error)
}

var StsClient StsProviderAPI

type ByocAws struct {
	*byoc.ByocBaseClient

	driver *cfn.AwsEcs // TODO: ecs is stateful, contains the output of the cd cfn stack after setUpCD

	ecsEventHandlers []ECSEventHandler
	handlersLock     sync.RWMutex
	cdEtag           types.ETag
	cdStart          time.Time
	cdTaskArn        ecs.TaskArn
}

var _ client.Provider = (*ByocAws)(nil)

type ErrMissingAwsCreds struct {
	err error
}

func (e ErrMissingAwsCreds) Error() string {
	return "Could not authenticate to the AWS service. Please check your AWS credentials and try again. (https://docs.defang.io/docs/providers/aws/#getting-started)"
}

func (e ErrMissingAwsCreds) Unwrap() error {
	return e.err
}

type ErrMissingAwsRegion struct {
	err error
}

func (e ErrMissingAwsRegion) Error() string {
	return e.err.Error() + " (https://docs.defang.io/docs/providers/aws#region)"
}

func (e ErrMissingAwsRegion) Unwrap() error {
	return e.err
}

func AnnotateAwsError(err error) error {
	if err == nil {
		return nil
	}
	term.Debug("AWS error:", err)
	if strings.Contains(err.Error(), "missing AWS region:") {
		return ErrMissingAwsRegion{err}
	}
	if cerr := new(aws.ErrNoSuchKey); errors.As(err, &cerr) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if cerr := new(aws.ErrParameterNotFound); errors.As(err, &cerr) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if cerr := new(smithy.OperationError); errors.As(err, &cerr) {
		// Auth can fail for many reasons: ec2imds not available, timeout, no profile, etc. so only check for top-level STS error
		if cerr.ServiceID == "STS" {
			return ErrMissingAwsCreds{cerr}
		}
		return cerr.Err
	}
	if cerr := new(cwTypes.SessionStreamingException); errors.As(err, &cerr) {
		return connect.NewError(connect.CodeInternal, err)
	}
	return err
}

type NewByocInterface func(ctx context.Context, tenantName types.TenantName) *ByocAws

func newByocProvider(ctx context.Context, tenantName types.TenantName) *ByocAws {
	b := &ByocAws{
		driver: cfn.New(byoc.CdTaskPrefix, aws.Region("")), // default region
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(ctx, tenantName, b)

	return b
}

var NewByocProvider NewByocInterface = newByocProvider

func initStsClient(cfg awssdk.Config) {
	if StsClient == nil {
		StsClient = sts.NewFromConfig(cfg)
	}
}

func (b *ByocAws) setUpCD(ctx context.Context) error {
	if b.SetupDone {
		return nil
	}

	term.Debugf("Using CD image: %q", b.CDImage)

	cdTaskName := byoc.CdTaskPrefix
	containers := []types.Container{
		{
			// FIXME: get the Pulumi image or version from Fabric: https://github.com/DefangLabs/defang/issues/1027
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
			Image:     b.CDImage,
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
		return AnnotateAwsError(err)
	}

	b.SetupDone = true
	return nil
}

func (b *ByocAws) GetDeploymentStatus(ctx context.Context) error {
	return ecs.GetTaskStatus(ctx, b.cdTaskArn)
}

func (b *ByocAws) Deploy(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "up")
}

func (b *ByocAws) Preview(ctx context.Context, req *defangv1.DeployRequest) (*defangv1.DeployResponse, error) {
	return b.deploy(ctx, req, "preview")
}

func (b *ByocAws) deploy(ctx context.Context, req *defangv1.DeployRequest, cmd string) (*defangv1.DeployResponse, error) {
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}

	// If multiple Compose files were provided, req.Compose is the merged representation of all the files
	project, err := compose.LoadFromContent(ctx, req.Compose, "")
	if err != nil {
		return nil, err
	}

	if err := b.setUpCD(ctx); err != nil {
		return nil, err
	}

	quotaClient = NewServiceQuotasClient(cfg)
	if err = validateGPUResources(ctx, project); err != nil {
		return nil, err
	}

	etag := pkg.RandomID()
	serviceInfos, err := b.GetServiceInfos(ctx, project.Name, req.DelegateDomain, etag, project.Services)
	if err != nil {
		return nil, err
	}

	data, err := proto.Marshal(&defangv1.ProjectUpdate{
		CdVersion: b.CDImage,
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
	b.cdEtag = etag
	b.cdStart = time.Now()
	b.cdTaskArn = taskArn

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
		return "", AnnotateAwsError(err)
	}

	if roleARN != "" {
		initStsClient(cfg)
		creds := stscreds.NewAssumeRoleProvider(StsClient, roleARN)
		cfg.Credentials = awssdk.NewCredentialsCache(creds)
	}

	r53Client := route53.NewFromConfig(cfg)

	domain = dns.Normalize(strings.ToLower(domain))
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
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}
	r53Client := route53.NewFromConfig(cfg)

	projectDomain := b.GetProjectDomain(req.Project, req.DelegateDomain)
	nsServers, delegationSetId, err := prepareDomainDelegation(ctx, projectDomain, r53Client)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}
	resp := client.PrepareDomainDelegationResponse{
		NameServers:     nsServers,
		DelegationSetId: delegationSetId,
	}
	return &resp, nil
}

func (b *ByocAws) AccountInfo(ctx context.Context) (client.AccountInfo, error) {
	// Use STS to get the account ID
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}
	initStsClient(cfg)

	identity, err := StsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, AnnotateAwsError(err)
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

func (i AWSAccountInfo) Provider() client.ProviderID {
	return client.ProviderAWS
}

func (i AWSAccountInfo) Region() string {
	return i.region
}

func (i AWSAccountInfo) Details() string {
	return i.arn
}

func (b *ByocAws) GetService(ctx context.Context, s *defangv1.GetRequest) (*defangv1.ServiceInfo, error) {
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

func (b *ByocAws) environment(projectName string) (map[string]string, error) {
	region := b.driver.Region // TODO: this should be the destination region, not the CD region; make customizable
	defangStateUrl := fmt.Sprintf(`s3://%s?region=%s&awssdk=v2`, b.bucketName(), region)
	pulumiBackendKey, pulumiBackendValue, err := byoc.GetPulumiBackend(defangStateUrl)
	if err != nil {
		return nil, err
	}
	env := map[string]string{
		// "AWS_REGION":               region.String(), should be set by ECS (because of CD task role)
		"DEFANG_DEBUG":               os.Getenv("DEFANG_DEBUG"), // TODO: use the global DoDebug flag
		"DEFANG_ORG":                 b.TenantName,
		"DEFANG_PREFIX":              byoc.DefangPrefix,
		"DEFANG_STATE_URL":           defangStateUrl,
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",
		"PRIVATE_DOMAIN":             byoc.GetPrivateDomain(projectName),
		"PROJECT":                    projectName,                 // may be empty
		pulumiBackendKey:             pulumiBackendValue,          // TODO: make secret
		"PULUMI_CONFIG_PASSPHRASE":   byoc.PulumiConfigPassphrase, // TODO: make secret
		"PULUMI_SKIP_UPDATE_CHECK":   "true",
		"STACK":                      b.PulumiStack,
	}

	if !term.StdoutCanColor() {
		env["NO_COLOR"] = "1"
	}

	return env, nil
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
	env, err := b.environment(cmd.project)
	if err != nil {
		return nil, err
	}
	if cmd.delegationSetId != "" {
		env["DELEGATION_SET_ID"] = cmd.delegationSetId
	}
	if cmd.delegateDomain != "" {
		env["DOMAIN"] = b.GetProjectDomain(cmd.project, cmd.delegateDomain)
	} else {
		env["DOMAIN"] = "dummy.domain"
	}
	env["DEFANG_MODE"] = strings.ToLower(cmd.mode.String())

	if term.DoDebug() || os.Getenv("DEFANG_PULUMI_DIR") != "" {
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
	if err := b.setUpCD(ctx); err != nil {
		return nil, err
	}
	// FIXME: this should only delete the services that are specified in the request, not all
	cmd := cdCmd{
		mode:           defangv1.DeploymentMode_MODE_UNSPECIFIED,
		project:        req.Project,
		delegateDomain: req.DelegateDomain,
		cmd:            []string{"up", ""}, // 2nd empty string is a empty payload
	}
	taskArn, err := b.runCdCommand(ctx, cmd)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}
	etag := ecs.GetTaskID(taskArn) // TODO: this is the CD task ID, not the etag
	b.cdEtag = etag
	b.cdStart = time.Now()
	b.cdTaskArn = taskArn
	return &defangv1.DeleteResponse{Etag: etag}, nil
}

func (b *ByocAws) GetProjectUpdate(ctx context.Context, projectName string) (*defangv1.ProjectUpdate, error) {
	if projectName == "" {
		return nil, nil
	}
	bucketName := b.bucketName()
	if bucketName == "" {
		if err := b.driver.FillOutputs(ctx); err != nil {
			// FillOutputs might fail if the stack is not created yet; return empty update in that case
			var cfnErr *cfn.ErrStackNotFoundException
			if errors.As(err, &cfnErr) {
				term.Debugf("FillOutputs: %v", err)
				return nil, nil // no services yet
			}
			return nil, AnnotateAwsError(err)
		}
		bucketName = b.bucketName()
	}

	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}

	s3Client := s3.NewFromConfig(cfg)
	// Path to the state file, Defined at: https://github.com/DefangLabs/defang-mvp/blob/main/pulumi/cd/aws/byoc.ts#L104
	pkg.Ensure(projectName != "", "ProjectName not set")
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
		return nil, AnnotateAwsError(err)
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

func (b *ByocAws) GetServices(ctx context.Context, req *defangv1.GetServicesRequest) (*defangv1.GetServicesResponse, error) {
	projUpdate, err := b.GetProjectUpdate(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	listServiceResp := defangv1.GetServicesResponse{}
	if projUpdate != nil {
		listServiceResp.Services = projUpdate.Services
		listServiceResp.Project = projUpdate.Project
	}

	return &listServiceResp, nil
}

func (b *ByocAws) getSecretID(projectName, name string) string {
	return b.StackDir(projectName, name) // same as defang_service.ts
}

func (b *ByocAws) PutConfig(ctx context.Context, secret *defangv1.PutConfigRequest) error {
	if !pkg.IsValidSecretName(secret.Name) {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret name; must be alphanumeric or _, cannot start with a number: %q", secret.Name))
	}
	fqn := b.getSecretID(secret.Project, secret.Name)
	term.Debugf("Putting parameter %q", fqn)
	err := b.driver.PutSecret(ctx, fqn, secret.Value)
	return AnnotateAwsError(err)
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
	if err := b.setUpCD(ctx); err != nil {
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

func (b *ByocAws) QueryForDebug(ctx context.Context, req *defangv1.DebugRequest) error {
	// tailRequest := &defangv1.TailRequest{
	// 	Etag:     req.Etag,
	// 	Project:  req.Project,
	// 	Services: req.Services,
	// 	Since:    req.Since,
	// 	Until:    req.Until,
	// }

	// The LogStreamNamePrefix filter can only be used with one service name
	var service string
	if len(req.Services) == 1 {
		service = req.Services[0]
	}

	start := b.cdStart // TODO: get start time from req.Etag
	if req.Since.IsValid() {
		start = req.Since.AsTime()
	} else if start.IsZero() {
		start = time.Now().Add(-time.Hour)
	}

	end := time.Now()
	if req.Until.IsValid() {
		end = req.Until.AsTime()
	}

	// get stack information (for log group ARN)
	err := b.driver.FillOutputs(ctx)
	if err != nil {
		return AnnotateAwsError(err)
	}

	// Gather logs from the CD task, kaniko, ECS events, and all services
	evtsChan, errsChan := ecs.QueryLogGroups(ctx, start, end, b.getLogGroupInputs(req.Etag, req.Project, service, "", logs.LogTypeAll)...)
	if evtsChan == nil {
		return <-errsChan
	}

	const maxQuerySizePerLogGroup = 128 * 1024 // 128KB per LogGroup (to stay well below the 1MB gRPC payload limit)

	sb := strings.Builder{}
loop:
	for {
		select {
		case event, ok := <-evtsChan:
			if !ok {
				break loop
			}
			parseECSEventRecords := strings.HasSuffix(*event.LogGroupIdentifier, "/ecs")
			if parseECSEventRecords {
				if event, err := ecs.ParseECSEvent([]byte(*event.Message)); err == nil {
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
			msg := term.StripAnsi(*event.Message)
			sb.WriteString(msg)
			sb.WriteByte('\n')
			if sb.Len() > maxQuerySizePerLogGroup { // FIXME: this limit was supposed to be per LogGroup
				term.Warn("Query result was truncated")
				break loop
			}
		case err := <-errsChan:
			term.Warn("CloudWatch query error:", AnnotateAwsError(err))
			// continue reading other log groups
		}
	}
	req.Logs = sb.String()
	return nil
}

func (b *ByocAws) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	// FillOutputs is needed to get the CD task ARN or the LogGroup ARNs
	if err := b.driver.FillOutputs(ctx); err != nil {
		return nil, AnnotateAwsError(err)
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
	var tailStream ecs.LiveTailStream

	if etag != "" && !pkg.IsValidRandomID(etag) { // Assume invalid "etag" is a task ID
		tailStream, err = b.driver.TailTaskID(ctx, etag)
		if err == nil {
			b.cdTaskArn, err = b.driver.GetTaskArn(etag)
			etag = "" // no need to filter by etag
		}
	} else {
		var service string
		if len(req.Services) == 1 {
			service = req.Services[0]
		}
		var start, end time.Time
		if req.Since.IsValid() {
			start = req.Since.AsTime()
		}
		if req.Until.IsValid() {
			end = req.Until.AsTime()
		}
		tailStream, err = ecs.QueryAndTailLogGroups(ctx, start, end, b.getLogGroupInputs(etag, req.Project, service, req.Pattern, logs.LogType(req.LogType))...)
	}

	if err != nil {
		return nil, AnnotateAwsError(err)
	}

	return newByocServerStream(ctx, tailStream, etag, req.GetServices(), b), nil
}

func (b *ByocAws) makeLogGroupARN(name string) string {
	return b.driver.MakeARN("logs", "log-group:"+name)
}

func (b *ByocAws) getLogGroupInputs(etag types.ETag, projectName, service, filter string, logType logs.LogType) []ecs.LogGroupInput {
	// Escape the filter pattern to avoid problems with the CloudWatch Logs pattern syntax
	// See https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/FilterAndPatternSyntax.html
	var pattern string // TODO: add etag to filter
	if filter != "" {
		pattern = strconv.Quote(filter)
	}

	var groups []ecs.LogGroupInput
	// Tail CD and kaniko
	if logType.Has(logs.LogTypeBuild) {
		cdTail := ecs.LogGroupInput{LogGroupARN: b.driver.LogGroupARN, LogEventFilterPattern: pattern} // TODO: filter by etag
		// If we know the CD task ARN, only tail the logstream for that CD task
		if b.cdTaskArn != nil && b.cdEtag == etag {
			cdTail.LogStreamNames = []string{ecs.GetCDLogStreamForTaskID(ecs.GetTaskID(b.cdTaskArn))}
		}
		groups = append(groups, cdTail)
		term.Debug("Query CD logs", cdTail.LogGroupARN, cdTail.LogStreamNames, filter)
		kanikoTail := ecs.LogGroupInput{LogGroupARN: b.makeLogGroupARN(b.StackDir(projectName, "builds")), LogEventFilterPattern: pattern} // must match logic in ecs/common.ts; TODO: filter by etag/service
		term.Debug("Query kaniko logs", kanikoTail.LogGroupARN, filter)
		groups = append(groups, kanikoTail)
		ecsTail := ecs.LogGroupInput{LogGroupARN: b.makeLogGroupARN(b.StackDir(projectName, "ecs")), LogEventFilterPattern: pattern} // must match logic in ecs/common.ts; TODO: filter by etag/service/deploymentId
		term.Debug("Query ecs events logs", ecsTail.LogGroupARN, filter)
		groups = append(groups, ecsTail)
	}
	// Tail services
	if logType.Has(logs.LogTypeRun) {
		servicesTail := ecs.LogGroupInput{LogGroupARN: b.makeLogGroupARN(b.StackDir(projectName, "logs")), LogEventFilterPattern: pattern} // must match logic in ecs/common.ts
		if service != "" && etag != "" {
			servicesTail.LogStreamNamePrefix = service + "/" + service + "_" + etag
		}
		term.Debug("Query services logs", servicesTail.LogGroupARN, servicesTail.LogStreamNamePrefix, pattern)
		groups = append(groups, servicesTail)
	}
	return groups
}

func (b *ByocAws) UpdateServiceInfo(ctx context.Context, si *defangv1.ServiceInfo, projectName, delegateDomain string, service composeTypes.ServiceConfig) error {
	if service.DomainName == "" {
		return nil
	}
	// Do a DNS lookup for DomainName and confirm it's indeed a CNAME to the service's public FQDN
	cname, _ := net.LookupCNAME(service.DomainName)
	if dns.Normalize(cname) != si.PublicFqdn {
		dnsRole, _ := service.Extensions["x-defang-dns-role"].(string)
		zoneId, err := b.findZone(ctx, service.DomainName, dnsRole)
		if err != nil {
			return err
		}
		if zoneId == "" {
			si.UseAcmeCert = true
			// TODO: We should add link to documentation on how the acme cert workflow works
			// TODO: Should we make this the default behavior or require the user to set a flag?
		} else {
			si.ZoneId = zoneId
		}
	}
	return nil
}

func (b *ByocAws) TearDown(ctx context.Context) error {
	return b.driver.TearDown(ctx)
}

func (b *ByocAws) BootstrapCommand(ctx context.Context, req client.BootstrapCommandRequest) (string, error) {
	if err := b.setUpCD(ctx); err != nil {
		return "", err
	}
	cmd := cdCmd{
		mode:    defangv1.DeploymentMode_MODE_UNSPECIFIED,
		project: req.Project,
		cmd:     []string{req.Command},
	}
	cdTaskArn, err := b.runCdCommand(ctx, cmd) // TODO: make domain optional for defang cd
	if err != nil || cdTaskArn == nil {
		return "", AnnotateAwsError(err)
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
		return AnnotateAwsError(err)
	}
	return nil
}

type s3Obj struct{ obj s3types.Object }

func (a s3Obj) Name() string {
	return *a.obj.Key
}

func (a s3Obj) Size() int64 {
	return *a.obj.Size
}

func (b *ByocAws) BootstrapList(ctx context.Context) ([]string, error) {
	bucketName := b.bucketName()
	if bucketName == "" {
		if err := b.driver.FillOutputs(ctx); err != nil {
			return nil, AnnotateAwsError(err)
		}
		bucketName = b.bucketName()
	}

	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}

	s3client := s3.NewFromConfig(cfg)
	return ListPulumiStacks(ctx, s3client, bucketName)
}

func ListPulumiStacks(ctx context.Context, s3client *s3.Client, bucketName string) ([]string, error) {
	prefix := `.pulumi/stacks/` // TODO: should we filter on `projectName`?

	term.Debug("Listing stacks in bucket:", bucketName)
	out, err := s3client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: &bucketName,
		Prefix: &prefix,
	})
	if err != nil {
		return nil, AnnotateAwsError(err)
	}
	var stacks []string
	for _, obj := range out.Contents {
		if obj.Key == nil || obj.Size == nil {
			continue
		}
		stack, err := byoc.ParsePulumiStackObject(ctx, s3Obj{obj}, bucketName, prefix, func(ctx context.Context, bucket, path string) ([]byte, error) {
			getObjectOutput, err := s3client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: &bucket,
				Key:    &path,
			})
			if err != nil {
				return nil, err
			}
			return io.ReadAll(getObjectOutput.Body)
		})
		if err != nil {
			return nil, err
		}
		if stack != "" {
			stacks = append(stacks, stack)
		}
		// TODO: check for lock files
	}
	return stacks, nil
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
