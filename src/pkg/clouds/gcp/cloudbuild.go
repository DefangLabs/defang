package gcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	cloudbuildpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"go.yaml.in/yaml/v4"
	"google.golang.org/protobuf/types/known/durationpb"
)

type MachineType = cloudbuildpb.BuildOptions_MachineType

const (
	UNSPECIFIED   MachineType = cloudbuildpb.BuildOptions_UNSPECIFIED
	N1_HIGHCPU_8  MachineType = cloudbuildpb.BuildOptions_N1_HIGHCPU_8
	N1_HIGHCPU_32 MachineType = cloudbuildpb.BuildOptions_N1_HIGHCPU_32
	E2_HIGHCPU_8  MachineType = cloudbuildpb.BuildOptions_E2_HIGHCPU_8
	E2_HIGHCPU_32 MachineType = cloudbuildpb.BuildOptions_E2_HIGHCPU_32
	E2_MEDIUM     MachineType = cloudbuildpb.BuildOptions_E2_MEDIUM
)

const DefangCDBuildTag = "defang-cd"

type CloudBuildArgs struct {
	// Required fields
	Steps string

	// TODO: We should be able to use ETAG from object metadata as digest in Diff func to determine a new build is necessary
	// Optional fields
	Source         string
	Images         []string          `pulumi:"images,optional" provider:"replaceOnChanges"`
	ServiceAccount *string           `pulumi:"serviceAccount,optional" provider:"replaceOnChanges"`
	Tags           []string          `pulumi:"tags,optional"`
	MachineType    *string           `pulumi:"machineType,optional"`
	DiskSizeGb     *int64            `pulumi:"diskSizeGb,optional"`
	Substitutions  map[string]string `pulumi:"substitutions,optional"`
}

type BuildTag struct {
	Stack      string
	Project    string
	Service    string
	Etag       string
	IsDefangCD bool
}

func (bt BuildTag) String() string {
	if bt.Stack == "" {
		return fmt.Sprintf("%s_%s_%s", bt.Project, bt.Service, bt.Etag)
	} else {
		return fmt.Sprintf("%s_%s_%s_%s", bt.Stack, bt.Project, bt.Service, bt.Etag)
	}
}

func (bt *BuildTag) Parse(tags []string) error {
	for _, tag := range tags {
		if tag == DefangCDBuildTag {
			bt.IsDefangCD = true
			continue
		}
		parts := strings.Split(tag, "_")
		if len(parts) < 3 || len(parts) > 4 {
			return fmt.Errorf("invalid cloudbuild build tags value: %q", tag)
		}

		if len(parts) == 3 { // Backward compatibility
			bt.Stack = ""
			bt.Project = parts[0]
			bt.Service = parts[1]
			bt.Etag = parts[2]
		} else {
			bt.Stack = parts[0]
			bt.Project = parts[1]
			bt.Service = parts[2]
			bt.Etag = parts[3]
		}
	}
	return nil
}

func (gcp Gcp) GetBuildInfo(ctx context.Context, buildId string) (*BuildTag, error) {
	client, err := cloudbuild.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloudbuild client: %w", err)
	}
	defer client.Close()
	req := &cloudbuildpb.GetBuildRequest{
		ProjectId: gcp.ProjectId,
		Id:        buildId,
	}
	build, err := client.GetBuild(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get build: %w", err)
	}
	if build == nil {
		return nil, errors.New("build not found")
	}
	var bt BuildTag
	if err := bt.Parse(build.Tags); err != nil {
		return nil, fmt.Errorf("failed to parse build tags: %w", err)
	}
	if bt.Project != "" || bt.Service != "" || bt.Etag != "" {
		return &bt, nil
	}
	return nil, fmt.Errorf("cannot find build tag containing build info: %v", build.Tags)
}

func (gcp Gcp) RunCloudBuild(ctx context.Context, args CloudBuildArgs) (string, error) {
	client, err := cloudbuild.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create Cloud Build client: %w", err)
	}
	defer client.Close()

	var steps []*cloudbuildpb.BuildStep
	if err := yaml.Unmarshal([]byte(args.Steps), &steps); err != nil {
		return "", fmt.Errorf("failed to parse cloudbuild steps: %w, steps are:\n%v\n", err, args.Steps)
	}

	// TODO: Implement secrets with a global `availableSecrets` and per-step `secretEnv`
	// See: https://cloud.google.com/build/docs/securing-builds/use-secrets
	// TODO: Use inline secret for environment variables since there is no other way to pass env vars to build steps
	// var secrets *cloudbuildpb.Secrets

	// Create a build request
	build := &cloudbuildpb.Build{
		Substitutions: args.Substitutions,
		Steps:         steps,
		// TODO: Support NPM or Python packages using Artifacts field
		// AvailableSecrets: secrets,
		Options: &cloudbuildpb.BuildOptions{
			MachineType:             GetMachineType(args.MachineType),
			DiskSizeGb:              GetDiskSize(args.DiskSizeGb),
			Logging:                 cloudbuildpb.BuildOptions_CLOUD_LOGGING_ONLY,
			EnableStructuredLogging: true,
		},
		Timeout: durationpb.New(time.Hour),
		Tags:    args.Tags,
	}

	if args.Source != "" {
		// Extract bucket and object from the source
		bucket, object, err := parseGCSURI(args.Source)
		if err != nil {
			return "", fmt.Errorf("failed to parse source URI: %w", err)
		}
		build.Source = &cloudbuildpb.Source{
			Source: &cloudbuildpb.Source_StorageSource{
				StorageSource: &cloudbuildpb.StorageSource{
					Bucket: bucket,
					Object: object,
				},
			},
		}
	}

	if args.ServiceAccount != nil {
		build.ServiceAccount = fmt.Sprintf("projects/%s/serviceAccounts/%s", gcp.ProjectId, *args.ServiceAccount)
	}

	if args.Images != nil {
		build.Images = args.Images
	}

	// Trigger the build
	op, err := client.CreateBuild(ctx, &cloudbuildpb.CreateBuildRequest{
		ProjectId: gcp.ProjectId, // Replace with your GCP project ID
		// Current API endpoint does not support location
		// Parent:    fmt.Sprintf("projects/%s/locations/%s", args.ProjectId, args.Location),
		Build: build,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create build: %w", err)
	}

	return op.Name(), nil
}

func (gcp Gcp) GetBuildStatus(ctx context.Context, startBuildOpName string) error {
	svc, err := cloudbuild.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create Cloud Build client: %w", err)
	}
	defer svc.Close()

	op := svc.CreateBuildOperation(startBuildOpName)
	build, err := op.Poll(ctx)
	if err != nil {
		return fmt.Errorf("failed to poll build operation: %w", err)
	}
	if build != nil {
		if build.Status == cloudbuildpb.Build_SUCCESS {
			return io.EOF
		}
		return client.ErrDeploymentFailed{Message: fmt.Sprintf("build failed with status: %v", build.Status)}
	}
	return nil
}

func GetMachineType(machineType *string) MachineType {
	if machineType == nil {
		return UNSPECIFIED
	}
	m, ok := cloudbuildpb.BuildOptions_MachineType_value[*machineType]
	if !ok {
		return UNSPECIFIED
	}
	return MachineType(m)
}

func GetDiskSize(diskSizeGb *int64) int64 {
	if diskSizeGb == nil {
		return 0
	}
	return *diskSizeGb
}

func parseGCSURI(uri string) (bucket string, object string, err error) {
	if !strings.HasPrefix(uri, "gs://") {
		return "", "", errors.New("URI must start with 'gs://' prefix")
	}

	parts := strings.SplitN(uri[5:], "/", 2)
	if len(parts) < 2 {
		return "", "", errors.New("URI must contain a bucket and an object path")
	}
	if parts[0] == "" {
		return "", "", errors.New("bucket name cannot be empty")
	}
	obj, err := url.PathUnescape(parts[1]) // Because the base 64 encoding may contain '='
	if err != nil {
		return "", "", fmt.Errorf("failed to unescape object path: %w", err)
	}
	if obj == "" {
		return "", "", errors.New("object path cannot be empty")
	}

	return parts[0], obj, nil
}
