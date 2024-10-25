package compose

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/smithy-go/ptr"
	compose "github.com/compose-spec/compose-go/v2/types"
)

func TestValidationAndConvert(t *testing.T) {
	oldTerm := term.DefaultTerm
	t.Cleanup(func() {
		term.DefaultTerm = oldTerm
	})

	t.Setenv("NODE_ENV", "test") // for interpolate/compose.yaml

	testRunCompose(t, func(t *testing.T, path string) {
		logs := new(bytes.Buffer)
		term.DefaultTerm = term.NewTerm(logs, logs)

		options := LoaderOptions{ConfigPaths: []string{path}}
		loader := Loader{options: options}

		// this is all the configs that are used in the test compose files
		mockClient := validationMockClient{
			configs: []string{"CONFIG1", "CONFIG2", "dummy", "ENV1", "SENSITIVE_DATA"},
		}

		project, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		if err := ValidateProject(mockClient, project); err != nil {
			t.Logf("Project validation failed: %v", err)
			logs.WriteString(err.Error() + "\n")
		}

		if err := FixupServices(context.Background(), mockClient, project.Services, UploadModeIgnore); err != nil {
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

func TestValidateConfig(t *testing.T) {
	const ENV_VAR = "ENV_VAR"

	ctx := context.Background()
	mockClient := validationMockClient{}

	testProject := compose.Project{
		Services: compose.Services{},
	}
	t.Run("NOP", func(t *testing.T) {
		env := map[string]*string{
			ENV_VAR: ptr.String("blah"),
		}

		testProject.Services["service1"] = compose.ServiceConfig{Environment: env}
		if err := ValidateProjectConfig(ctx, mockClient, &testProject); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Missing Config", func(t *testing.T) {
		var missing ErrMissingConfig
		env := map[string]*string{
			ENV_VAR: ptr.String("blah"),
			"ASD":   nil,
			"BSD":   nil,
			"CSD":   nil,
		}

		ctx := context.Background()
		mockClient := validationMockClient{}
		testProject.Services["service1"] = compose.ServiceConfig{Environment: env}
		if err := ValidateProjectConfig(ctx, mockClient, &testProject); !errors.As(err, &missing) {
			t.Fatalf("uexpected ErrMissingConfig, got: %v", err)
		} else {
			if len(missing) != 3 {
				t.Fatalf("unexpected error: number of missing, got: %d expected 3", len(missing))
			}

			for index, name := range []string{"ASD", "BSD", "CSD"} {
				if missing[index] != name {
					t.Fatalf("unexpected error: missing, got: %s expected ASD", missing[index])
				}
			}
		}
	})

	t.Run("Valid Config", func(t *testing.T) {
		const CONFIG_VAR = "CONFIG_VAR"
		mockClient.configs = []string{CONFIG_VAR}
		env := map[string]*string{
			ENV_VAR:    ptr.String("blah"),
			CONFIG_VAR: nil,
		}
		testProject.Services["service1"] = compose.ServiceConfig{Environment: env}
		if err := ValidateProjectConfig(ctx, mockClient, &testProject); err != nil {
			t.Fatal(err)
		}
	})
}

type validationMockClient struct {
	client.Client
	configs []string
}

func (m validationMockClient) ListConfig(ctx context.Context) (*defangv1.Secrets, error) {
	return &defangv1.Secrets{
		Names:   m.configs,
		Project: "mock-project",
	}, nil
}

func (m validationMockClient) ServiceDNS(name string) string {
	return "mock-" + name
}
