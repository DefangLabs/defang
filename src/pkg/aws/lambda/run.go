package lambda

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/defang-io/defang/src/pkg/aws/ecs"
)

func (a *AwsLambda) Run(ctx context.Context, env map[string]string, cmd ...string) (ecs.TaskArn, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	io, err := lambda.NewFromConfig(cfg).Invoke(ctx, &lambda.InvokeInput{
		FunctionName:   &a.FunctionName,
		InvocationType: types.InvocationTypeEvent,
		LogType:        types.LogTypeTail,
		// Payload:        []byte(""),
		// ClientContext: nil,
	})
	if err != nil {
		return nil, err
	}
	io.
	return nil, nil
}

func (a *AwsLambda) Stop(ctx context.Context, task ecs.TaskArn) error {
	return nil
}

func (a *AwsLambda) Tail(ctx context.Context, task ecs.TaskArn) error {
	return nil
}

func (a *AwsLambda) Info(ctx context.Context, task ecs.TaskArn) (string, error) {
	return "", nil
}
