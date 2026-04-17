package codebuild

import (
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
	RetainBucket bool   // CloudFormation template input parameter
}
