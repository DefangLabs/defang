package cdk

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type DefangCdkStackProps struct {
	awscdk.StackProps
}

func newDefangCdkStack(scope constructs.Construct, id string, props *DefangCdkStackProps) awscdk.Stack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	// awss3.NewBucket(stack, jsii.String("MyFirstBucket"), &awss3.BucketProps{
	// 	Versioned: jsii.Bool(true),
	// })
	awsecs.NewCluster(stack, jsii.String("MyFirstCluster"), &awsecs.ClusterProps{})

	return stack
}

func Run() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	newDefangCdkStack(app, "DefangCdkStack", &DefangCdkStackProps{
		awscdk.StackProps{
			Env: env(),
		},
	})

	app.Synth(nil)
}

func env() *awscdk.Environment {
	return nil
}
