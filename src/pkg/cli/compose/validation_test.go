package compose

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestValidation(t *testing.T) {
	oldOut := logrus.StandardLogger().Out
	t.Cleanup(func() {
		logrus.SetOutput(oldOut)
	})

	testRunCompose(t, func(t *testing.T, path string) {
		logs := new(bytes.Buffer)
		logrus.SetOutput(logs)

		loader := Loader{path}
		proj, err := loader.LoadCompose(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateProject(proj); err != nil {
			t.Logf("Project validation failed: %v", err)
			logs.WriteString(err.Error() + "\n")
		}

		// The order of the services is not guaranteed, so we sort the logs before comparing
		logLines := strings.Split(strings.TrimSpace(logs.String()), "\n")
		slices.Sort(logLines)
		logs = bytes.NewBufferString(strings.Join(logLines, "\n"))

		// Compare the logs with the warnings file
		if err := compare(logs.Bytes(), path+".warnings"); err != nil {
			t.Error(err)
		}
	})
}
