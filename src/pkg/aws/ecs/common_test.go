package ecs

import (
	"testing"
)

func TestGetAccountID(t *testing.T) {
	a := AwsEcs{
		TaskDefARN: "arn:aws:ecs:us-east-1:123456789012:task-definition/defang-ecs-2021-08-31-163042",
	}
	if got := a.getAccountID(); got != "123456789012" {
		t.Errorf("GetAccountID() = %v, want 123456789012", got)
	}
}
