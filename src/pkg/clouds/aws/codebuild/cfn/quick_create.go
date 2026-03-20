package cfn

import "net/url"

type QuickCreateArgs struct {
	Region    string
	StackName string
	Params    map[string]string
}

func MakeQuickCreateURL(templateURL string, args QuickCreateArgs) (string, error) {
	var quickCreateUrl url.URL
	quickCreateUrl.Scheme = "https"
	quickCreateUrl.Host = args.Region + ".console.aws.amazon.com"
	quickCreateUrl.Path = "/cloudformation/home"
	quickCreateUrl.RawQuery = "region=" + args.Region
	fragment := url.Values{
		"templateURL": []string{templateURL},
	}
	if args.StackName != "" {
		fragment.Set("stackName", args.StackName)
	}
	for k, v := range args.Params {
		fragment.Set("param_"+k, v)
	}
	// We cannot use url.Fragment because it escapes the templateURL once more
	return quickCreateUrl.String() + "#/stacks/create/review?" + fragment.Encode(), nil
}
