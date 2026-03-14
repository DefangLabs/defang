package codebuild

import (
	"strings"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
)

const (
	DockerRegistry    = "docker.io"
	EcrPublicRegistry = "public.ecr.aws"
	CrunProjectName   = "defang"
)

type BuildID *string

type AwsCodeBuild struct {
	aws.Aws
	BucketName        string
	CIRoleARN         string
	DockerCachePrefix string // ECR pull-through cache prefix for Docker Hub (empty if not configured)
	EcrCachePrefix    string // ECR pull-through cache prefix for public ECR (empty if not configured)
	LogGroupARN       string
	ProjectName       string // CodeBuild project name
	RetainBucket      bool
}

func (a *AwsCodeBuild) MakeARN(service, resource string) string {
	return strings.Join([]string{
		"arn",
		"aws",
		service,
		string(a.Region),
		a.AccountID,
		resource,
	}, ":")
}
