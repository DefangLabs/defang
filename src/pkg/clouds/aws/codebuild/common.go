package codebuild

import (
	"strings"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
)

const (
	CrunProjectName = "defang"
)

type BuildID *string

type AwsCodeBuild struct {
	aws.Aws
	BucketName   string
	CIRoleARN    string
	LogGroupARN  string
	ProjectName  string // CodeBuild project name
	RetainBucket bool
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
