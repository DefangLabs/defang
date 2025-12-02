package aws

import "testing"

func TestGetAccountID(t *testing.T) {
	logGroupARN := "arn:aws:logs:us-east-1:123456789012:log-group:/defang-ecs-2021-08-31-163042"
	if got := GetAccountID(logGroupARN); got != "123456789012" {
		t.Errorf("GetAccountID() = %v, want 123456789012", got)
	}
}
