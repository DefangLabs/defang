package aws

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"iter"
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
	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
	"github.com/DefangLabs/defang/src/pkg/dns"
	"github.com/DefangLabs/defang/src/pkg/http"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/timeutils"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/bufbuild/connect-go"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"google.golang.org/protobuf/proto"
)

type StsProviderAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
	AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error)
}

var StsClient StsProviderAPI

type ByocAws struct {
	*byoc.ByocBaseClient

	driver *cfn.AwsEcsCfn // TODO: ecs is stateful, contains the output of the cd cfn stack after SetUpCD

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

func NewByocProvider(ctx context.Context, tenantName types.TenantName, stack string) *ByocAws {
	b := &ByocAws{
		driver: cfn.New(byoc.CdTaskPrefix, aws.Region("")), // default region
	}
	b.ByocBaseClient = byoc.NewByocBaseClient(tenantName, b, stack)

	return b
}

func initStsClient(cfg awssdk.Config) {
	if StsClient == nil {
		StsClient = sts.NewFromConfig(cfg)
	}
}

func (b *ByocAws) makeContainers() []clouds.Container {
	return makeContainers(b.PulumiVersion, b.CDImage)
}

func (b *ByocAws) PrintCloudFormationTemplate() ([]byte, error) {
	containers := b.makeContainers()
	template, err := cfn.CreateTemplate(byoc.CdTaskPrefix, containers)
	if err != nil {
		return nil, err
	}
	return template.YAML()
}

func (b *ByocAws) SetUpCD(ctx context.Context) error {
	if b.SetupDone {
		return nil
	}

	term.Debugf("Using CD image: %q", b.CDImage)

	if err := b.driver.SetUp(ctx, b.makeContainers()); err != nil {
		return AnnotateAwsError(err)
	}

	// Delete default SecurityGroup rules to comply with stricter AWS account security policies
	if sgId := b.driver.DefaultSecurityGroupID; sgId != "" {
		term.Debugf("Cleaning up default Security Group rules (%s)", sgId)
		if err := b.driver.RevokeDefaultSecurityGroupRules(ctx, sgId); err != nil {
			term.Warnf("Could not clean up default Security Group rules: %v", err)
		}
	}

	b.SetupDone = true
	return nil
}

func (b *ByocAws) GetDeploymentStatus(ctx context.Context) error {
	if err := ecs.GetTaskStatus(ctx, b.cdTaskArn); err != nil {
		// check if the task failed; if so, return the a ErrDeploymentFailed error
		if taskErr := new(ecs.TaskFailure); errors.As(err, taskErr) {
			return client.ErrDeploymentFailed{Message: taskErr.Error()}
		}
		return err
	}
	return nil
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

	if err := b.SetUpCD(ctx); err != nil {
		return nil, err
	}

	quotaClient = NewServiceQuotasClient(cfg)
	if err = validateGPUResources(ctx, project); err != nil {
		return nil, err
	}

	etag := types.NewEtag()
	serviceInfos, err := b.GetServiceInfos(ctx, project.Name, req.DelegateDomain, etag, project.Services)
	if err != nil {
		return nil, err
	}

	data, err := proto.Marshal(&defangv1.ProjectUpdate{
		CdVersion: b.CDImage,
		Compose:   req.Compose,
		Mode:      req.Mode,
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

	cdCmd := cdCommand{
		command:         []string{cmd, payloadString},
		delegateDomain:  req.DelegateDomain,
		delegationSetId: req.DelegationSetId,
		mode:            req.Mode,
		project:         project.Name,
	}
	cdTaskArn, err := b.runCdCommand(ctx, cdCmd)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}
	b.cdEtag = etag
	b.cdStart = time.Now()
	b.cdTaskArn = cdTaskArn

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
		zones, err := aws.GetHostedZonesByName(ctx, domain, r53Client)
		if errors.Is(err, aws.ErrZoneNotFound) {
			if strings.Count(domain, ".") <= 1 {
				return "", nil
			}
			domain = domain[strings.Index(domain, ".")+1:]
			continue
		} else if err != nil {
			return "", err
		}
		if len(zones) > 1 {
			term.Warnf("Multiple hosted zones found for domain %q, using the first one: %v", domain, zones[0].Id)
		}
		return *zones[0].Id, nil
	}
}

func (b *ByocAws) PrepareDomainDelegation(ctx context.Context, req client.PrepareDomainDelegationRequest) (*client.PrepareDomainDelegationResponse, error) {
	cfg, err := b.driver.LoadConfig(ctx)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}
	r53Client := route53.NewFromConfig(cfg)

	projectDomain := b.GetProjectDomain(req.Project, req.DelegateDomain)
	nsServers, delegationSetId, err := prepareDomainDelegation(ctx, projectDomain, req.Project, b.PulumiStack, r53Client, dns.ResolverAt)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}
	resp := client.PrepareDomainDelegationResponse{
		NameServers:     nsServers,
		DelegationSetId: delegationSetId,
	}
	return &resp, nil
}

func (b *ByocAws) AccountInfo(ctx context.Context) (*client.AccountInfo, error) {
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

	return &client.AccountInfo{
		Region:    cfg.Region,
		AccountID: *identity.Account,
		Details:   *identity.Arn, // contains the user role
		Provider:  client.ProviderAWS,
	}, nil
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
		"DEFANG_JSON":                os.Getenv("DEFANG_JSON"),
		"DEFANG_ORG":                 b.TenantName,
		"DEFANG_PREFIX":              b.Prefix,
		"DEFANG_STATE_URL":           defangStateUrl,
		"NODE_NO_WARNINGS":           "1",
		"NPM_CONFIG_UPDATE_NOTIFIER": "false",
		"PRIVATE_DOMAIN":             byoc.GetPrivateDomain(projectName),
		"PROJECT":                    projectName,                 // may be empty
		pulumiBackendKey:             pulumiBackendValue,          // TODO: make secret
		"PULUMI_CONFIG_PASSPHRASE":   byoc.PulumiConfigPassphrase, // TODO: make secret
		"PULUMI_COPILOT":             "false",
		"PULUMI_SKIP_UPDATE_CHECK":   "true",
		"STACK":                      b.PulumiStack,
	}

	if !term.StdoutCanColor() {
		env["NO_COLOR"] = "1"
	}

	return env, nil
}

type cdCommand struct {
	command         []string
	delegateDomain  string
	delegationSetId string
	mode            defangv1.DeploymentMode
	project         string
}

func (b *ByocAws) runCdCommand(ctx context.Context, cmd cdCommand) (ecs.TaskArn, error) {
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

	if os.Getenv("DEFANG_PULUMI_DIR") != "" {
		// Convert the environment to a human-readable array of KEY=VALUE strings for debugging
		debugEnv := []string{"AWS_REGION=" + b.driver.Region.String()}
		if awsProfile := os.Getenv("AWS_PROFILE"); awsProfile != "" {
			debugEnv = append(debugEnv, "AWS_PROFILE="+awsProfile)
		}
		for k, v := range env {
			debugEnv = append(debugEnv, k+"="+v)
		}
		if err := byoc.DebugPulumiNodeJS(ctx, debugEnv, cmd.command...); err != nil {
			return nil, err
		}
	}
	return b.driver.Run(ctx, env, cmd.command...)
}

func (b *ByocAws) Delete(ctx context.Context, req *defangv1.DeleteRequest) (*defangv1.DeleteResponse, error) {
	if len(req.Names) > 0 {
		return nil, client.ErrNotImplemented("per-service deletion is not supported for BYOC")
	}
	if err := b.SetUpCD(ctx); err != nil {
		return nil, err
	}
	// FIXME: this should only delete the services that are specified in the request, not all
	cmd := cdCommand{
		project:        req.Project,
		delegateDomain: req.DelegateDomain,
		command:        []string{"up", ""}, // 2nd empty string is a empty payload
	}
	cdTaskArn, err := b.runCdCommand(ctx, cmd)
	if err != nil {
		return nil, AnnotateAwsError(err)
	}
	etag := ecs.GetTaskID(cdTaskArn) // TODO: this is the CD task ID, not the etag
	b.cdEtag = etag
	b.cdStart = time.Now()
	b.cdTaskArn = cdTaskArn
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
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid config name; must be alphanumeric or _, cannot start with a number: %q", secret.Name))
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
	if err := b.SetUpCD(ctx); err != nil {
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

	// get stack information (for CD log group ARN)
	err := b.driver.FillOutputs(ctx)
	if err != nil {
		return AnnotateAwsError(err)
	}

	cw, err := ecs.NewCloudWatchLogsClient(ctx, b.driver.Region) // assume all log groups are in the same region
	if err != nil {
		return err
	}

	// Gather logs from the CD task, builds, ECS events, and all services
	evtsChan, errsChan := ecs.QueryLogGroups(ctx, cw, start, end, 0, b.getLogGroupInputs(req.Etag, req.Project, service, "", logs.LogTypeAll)...)
	if evtsChan == nil {
		return <-errsChan // TODO: there could be multiple errors
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
	// if the cloud formation stack has been destroyed, we can still query
	// logs for builds and services
	if err := b.driver.FillOutputs(ctx); err != nil {
		term.Warnf("Unable to show CD logs: %v", err) // TODO: could skip this warning if the user wasn't asking for CD logs
	}

	var err error
	cw, err := ecs.NewCloudWatchLogsClient(ctx, b.driver.Region) // assume all log groups are in the same region
	if err != nil {
		return nil, AnnotateAwsError(err)
	}

	// How to tail multiple tasks/services at once?
	//  * No Etag, no service:	tail all tasks/services
	//  * Etag, no service: 	tail all tasks/services with that Etag
	//  * No Etag, service:		tail all tasks/services with that service name
	//  * Etag, service:		tail that task/service
	var tailStream ecs.LiveTailStream
	etag := req.Etag
	if etag != "" && !pkg.IsValidRandomID(etag) { // Assume invalid "etag" is the task ID of the CD task
		tailStream, err = b.queryCdLogs(ctx, cw, req)
		etag = "" // no need to filter events by etag because we only show logs from the specified task ID
	} else {
		tailStream, err = b.queryLogs(ctx, cw, req)
	}
	if err != nil {
		return nil, AnnotateAwsError(err)
	}
	return newByocServerStream(tailStream, etag, req.Services, b), nil
}

func (b *ByocAws) queryCdLogs(ctx context.Context, cw *cloudwatchlogs.Client, req *defangv1.TailRequest) (ecs.LiveTailStream, error) {
	var err error
	b.cdTaskArn, err = b.driver.GetTaskArn(req.Etag) // only fails on missing task ID
	if err != nil {
		return nil, err
	}
	if req.Follow {
		return b.driver.TailTaskID(ctx, cw, req.Etag)
	} else {
		start := timeutils.AsTime(req.Since, time.Time{})
		end := timeutils.AsTime(req.Until, time.Time{})
		return b.driver.QueryTaskID(ctx, cw, req.Etag, start, end, req.Limit)
	}
}

func (b *ByocAws) queryLogs(ctx context.Context, cw *cloudwatchlogs.Client, req *defangv1.TailRequest) (ecs.LiveTailStream, error) {
	start := timeutils.AsTime(req.Since, time.Time{})
	end := timeutils.AsTime(req.Until, time.Time{})

	var service string
	if len(req.Services) == 1 {
		service = req.Services[0]
	}
	lgis := b.getLogGroupInputs(req.Etag, req.Project, service, req.Pattern, logs.LogType(req.LogType))
	if req.Follow {
		return ecs.QueryAndTailLogGroups(
			ctx,
			cw,
			start,
			end,
			lgis...,
		)
	} else {
		evtsChan, errsChan := ecs.QueryLogGroups(
			ctx,
			cw,
			start,
			end,
			req.Limit,
			lgis...,
		)
		if evtsChan == nil {
			var errs []error
			for err := range errsChan {
				errs = append(errs, err)
			}
			return nil, errors.Join(errs...)
		}
		// TODO: any errors from errsChan should be reported but get dropped
		return ecs.NewStaticLogStream(evtsChan, func() {}), nil
	}
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
	// Tail CD and builds
	if logType.Has(logs.LogTypeBuild) {
		if b.driver.LogGroupARN == "" {
			term.Debug("CD stack LogGroupARN is not set; skipping CD logs")
		} else {
			cdTail := ecs.LogGroupInput{LogGroupARN: b.driver.LogGroupARN, LogEventFilterPattern: pattern} // TODO: filter by etag
			// If we know the CD task ARN, only tail the logstream for that CD task
			if b.cdTaskArn != nil && b.cdEtag == etag {
				cdTail.LogStreamNames = []string{ecs.GetCDLogStreamForTaskID(ecs.GetTaskID(b.cdTaskArn))}
			}
			groups = append(groups, cdTail)
			term.Debug("Query CD logs", cdTail.LogGroupARN, cdTail.LogStreamNames, filter)
		}
		buildsTail := ecs.LogGroupInput{LogGroupARN: b.makeLogGroupARN(b.StackDir(projectName, "builds")), LogEventFilterPattern: pattern} // must match logic in ecs/common.ts; TODO: filter by etag/service
		term.Debug("Query builds logs", buildsTail.LogGroupARN, filter)
		groups = append(groups, buildsTail)
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
		// Set the ZoneId so CD can manage any records for us
		si.ZoneId = zoneId
	}
	return nil
}

func (b *ByocAws) TearDownCD(ctx context.Context) error {
	return b.driver.TearDown(ctx)
}

func (b *ByocAws) BootstrapCommand(ctx context.Context, req client.BootstrapCommandRequest) (string, error) {
	if err := b.SetUpCD(ctx); err != nil {
		return "", err
	}
	cmd := cdCommand{
		project: req.Project,
		command: []string{req.Command},
	}
	cdTaskArn, err := b.runCdCommand(ctx, cmd) // TODO: make domain optional for defang cd
	if err != nil {
		return "", AnnotateAwsError(err)
	}
	etag := ecs.GetTaskID(cdTaskArn) // TODO: this is the CD task ID, not the etag
	b.cdEtag = etag
	b.cdStart = time.Now()
	b.cdTaskArn = cdTaskArn
	return etag, nil
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

func (b *ByocAws) BootstrapList(ctx context.Context, allRegions bool) (iter.Seq[string], error) {
	if allRegions {
		s3Client, err := newS3Client(ctx, b.driver.Region)
		if err != nil {
			return nil, AnnotateAwsError(err)
		}
		return listPulumiStacksAllRegions(ctx, s3Client)
	} else {
		bucketName := b.bucketName()
		if bucketName == "" {
			if err := b.driver.FillOutputs(ctx); err != nil {
				return nil, AnnotateAwsError(err)
			}
			bucketName = b.bucketName()
		}
		return listPulumiStacksInBucket(ctx, b.driver.Region, bucketName)
	}
}

type ECSEventHandler interface {
	HandleECSEvent(evt ecs.Event)
}

func (b *ByocAws) Subscribe(ctx context.Context, req *defangv1.SubscribeRequest) (client.ServerStream[defangv1.SubscribeResponse], error) {
	s := &byocSubscribeServerStream{
		services: req.Services,
		etag:     req.Etag,

		ch:   make(chan *defangv1.SubscribeResponse),
		done: make(chan struct{}),
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

func (b *ByocAws) ServicePublicDNS(name string, projectName string) string {
	return dns.SafeLabel(name) + "." + dns.SafeLabel(projectName) + "." + dns.SafeLabel(b.TenantName) + ".defang.app"
}
