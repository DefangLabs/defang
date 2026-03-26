package codebuild

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cbtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
)

// jsonBuild mirrors the subset of the AWS JSON wire format we need for testing.
type jsonBuild struct {
	BuildStatus string `json:"buildStatus"`
	Phases      []struct {
		PhaseType   string `json:"phaseType"`
		PhaseStatus string `json:"phaseStatus"`
		Contexts    []struct {
			Message    string `json:"message"`
			StatusCode string `json:"statusCode"`
		} `json:"contexts"`
	} `json:"phases"`
}

func loadBuild(t *testing.T, path string) cbtypes.Build {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Builds []jsonBuild `json:"builds"`
	}
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Builds) == 0 {
		t.Fatal("no builds in JSON")
	}
	jb := output.Builds[0]
	build := cbtypes.Build{
		BuildStatus: cbtypes.StatusType(jb.BuildStatus),
	}
	for _, jp := range jb.Phases {
		phase := cbtypes.BuildPhase{
			PhaseType:   cbtypes.BuildPhaseType(jp.PhaseType),
			PhaseStatus: cbtypes.StatusType(jp.PhaseStatus),
		}
		for _, jc := range jp.Contexts {
			phase.Contexts = append(phase.Contexts, cbtypes.PhaseContext{
				Message:    aws.String(jc.Message),
				StatusCode: aws.String(jc.StatusCode),
			})
		}
		build.Phases = append(build.Phases, phase)
	}
	return build
}

func TestGetBuildPhaseErrorContexts_FailedBuild(t *testing.T) {
	build := loadBuild(t, "testdata/codebuild-failed.json")

	actual := getBuildPhaseErrorContexts(build)
	expected := "Error while executing command: docker buildx build -t 123456789012.dkr.ecr.us-test-2.amazonaws.com/html-css-js/kaniko-build:app-image-103b5989-x86_64 -f Dockerfile --push --platform linux/amd64  ${CODEBUILD_SRC_DIR}. Reason: exit status 1"

	if actual != expected {
		t.Errorf("getBuildPhaseErrorContexts() = %q, want %q", actual, expected)
	}
}

func TestGetBuildPhaseErrorContexts_StoppedBuild(t *testing.T) {
	build := loadBuild(t, "testdata/codebuild-stopped.json")

	actual := getBuildPhaseErrorContexts(build)
	if actual != "" {
		t.Errorf("getBuildPhaseErrorContexts() = %q, want empty string", actual)
	}
}

func TestBuildStatus_Failed(t *testing.T) {
	build := loadBuild(t, "testdata/codebuild-failed.json")

	done, err := buildStatus(build)
	if !done {
		t.Error("expected done=true for failed build")
	}
	bf, ok := err.(BuildFailure)
	if !ok {
		t.Fatalf("expected BuildFailure, got %T", err)
	}
	expected := "Error while executing command: docker buildx build -t 123456789012.dkr.ecr.us-test-2.amazonaws.com/html-css-js/kaniko-build:app-image-103b5989-x86_64 -f Dockerfile --push --platform linux/amd64  ${CODEBUILD_SRC_DIR}. Reason: exit status 1"
	if bf.Reason != expected {
		t.Errorf("reason = %q, want %q", bf.Reason, expected)
	}
}

func TestBuildStatus_Stopped(t *testing.T) {
	build := loadBuild(t, "testdata/codebuild-stopped.json")

	done, err := buildStatus(build)
	if !done {
		t.Error("expected done=true for stopped build")
	}
	bf, ok := err.(BuildFailure)
	if !ok {
		t.Fatalf("expected BuildFailure, got %T", err)
	}
	if bf.Reason != "build stopped" {
		t.Errorf("reason = %q, want %q", bf.Reason, "build stopped")
	}
}

func TestBuildStatus_Succeeded(t *testing.T) {
	done, err := buildStatus(cbtypes.Build{BuildStatus: cbtypes.StatusTypeSucceeded})
	if !done {
		t.Error("expected done=true")
	}
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestBuildStatus_InProgress(t *testing.T) {
	done, err := buildStatus(cbtypes.Build{BuildStatus: cbtypes.StatusTypeInProgress})
	if done {
		t.Error("expected done=false")
	}
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestBuildStatus_TimedOut(t *testing.T) {
	done, err := buildStatus(cbtypes.Build{BuildStatus: cbtypes.StatusTypeTimedOut})
	if !done {
		t.Error("expected done=true")
	}
	bf, ok := err.(BuildFailure)
	if !ok {
		t.Fatalf("expected BuildFailure, got %T", err)
	}
	if bf.Reason != "build timed out" {
		t.Errorf("reason = %q, want %q", bf.Reason, "build timed out")
	}
}
