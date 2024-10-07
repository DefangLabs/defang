package compose

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestValidationAndConvert(t *testing.T) {
	oldTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = oldTerm
	})

	testRunCompose(t, func(t *testing.T, path string) {
		logs := new(bytes.Buffer)
		term.DefaultTerm = term.NewTerm(logs, logs)

		options := LoaderOptions{ConfigPaths: []string{path}}
		loader := Loader{options: options}
		proj, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateProject(proj); err != nil {
			t.Logf("Project validation failed: %v", err)
			logs.WriteString(err.Error() + "\n")
		}

		mockClient := MockClient{
			configs: []string{"CONFIG1", "CONFIG2"},
		}
		if _, err = ConvertServices(context.Background(), mockClient, proj.Services, BuildContextIgnore); err != nil {
			t.Logf("Service conversion failed: %v", err)
			logs.WriteString(err.Error() + "\n")
		}

		// The order of the services is not guaranteed, so we sort the logs before comparing
		logLines := strings.Split(strings.Trim(logs.String(), "\n"), "\n")
		slices.Sort(logLines)
		logs = bytes.NewBufferString(strings.Join(logLines, "\n"))

		// Compare the logs with the warnings file
		if err := compare(logs.Bytes(), path+".warnings"); err != nil {
			t.Error(err)
		}
	})
}

type MockClient struct {
	client.Client
	configs []string
}

func (m MockClient) ListConfigs(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.ListConfigsResponse, error) {
	configKeys := make([]*defangv1.ConfigKey, len(m.configs))

	for i, config := range m.configs {
		configKeys[i] = &defangv1.ConfigKey{
			Name:    config,
			Project: "mock-project",
		}
	}

	return &defangv1.ListConfigsResponse{Configs: configKeys}, nil
}

func (m MockClient) ServiceDNS(name string) string {
	return "mock-" + name
}

func (m MockClient) LoadProjectName(ctx context.Context) (string, error) {
	return "project1", nil
}
