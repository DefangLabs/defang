package cfn

import (
	"testing"
)

func TestMakeQuickCreateURL(t *testing.T) {
	// This example comes from https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/cfn-console-create-stacks-quick-create-links.html
	// but the parameters in the expected URL were sorted to match the Go behavior.
	quickCreateArgs := QuickCreateArgs{
		Region:    "us-test-2",
		StackName: "MyWPBlog",
		Params: map[string]string{
			"DBName":       "mywpblog",
			"InstanceType": "t2.medium",
		},
	}
	url, err := MakeQuickCreateURL("https://s3.us-test-2.amazonaws.com/cloudformation-templates-us-test-2/WordPress_Single_Instance.template", quickCreateArgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const expected = "https://us-test-2.console.aws.amazon.com/cloudformation/home?region=us-test-2#/stacks/create/review?param_DBName=mywpblog&param_InstanceType=t2.medium&stackName=MyWPBlog&templateURL=https%3A%2F%2Fs3.us-test-2.amazonaws.com%2Fcloudformation-templates-us-test-2%2FWordPress_Single_Instance.template"
	if url != expected {
		t.Errorf("expected URL\n%v\n, got\n%v", expected, url)
	}
}
