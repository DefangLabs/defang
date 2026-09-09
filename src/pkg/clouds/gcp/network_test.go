package gcp

import (
	"errors"
	"strings"
	"testing"

	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

func TestResourceName(t *testing.T) {
	for input, want := range map[string]string{
		"https://www.googleapis.com/compute/v1/projects/p/global/networks/proj-vpc-abc": "proj-vpc-abc",
		"projects/p/regions/us-central1/subnetworks/proj-subnet-def":                    "proj-subnet-def",
		"proj-vpc-abc": "proj-vpc-abc",
	} {
		if got := ResourceName(input); got != want {
			t.Errorf("ResourceName(%q) = %q, want %q", input, got, want)
		}
	}
}

// The compute API is inconsistent about returning full self-links versus bare paths for the same
// resource, so comparison has to be by name.
func TestSameResource(t *testing.T) {
	full := "https://www.googleapis.com/compute/v1/projects/p/global/networks/proj-vpc-abc"
	path := "projects/p/global/networks/proj-vpc-abc"
	if !sameResource(full, path) {
		t.Error("a self-link and a bare path for one resource should match")
	}
	if !sameResource(full, "proj-vpc-abc") {
		t.Error("a self-link and a bare name for one resource should match")
	}
	if sameResource(full, "projects/p/global/networks/other-vpc-abc") {
		t.Error("different resources should not match")
	}
	// An empty field means "not set", which must never match anything: Cloud Run leaves either
	// network or subnetwork empty and derives the other.
	if sameResource("", "") || sameResource(full, "") || sameResource("", full) {
		t.Error("an empty self-link must not match")
	}
}

func TestSubnetworkRegion(t *testing.T) {
	got := subnetworkRegion("https://www.googleapis.com/compute/v1/projects/p/regions/us-central1/subnetworks/s")
	if got != "us-central1" {
		t.Errorf("got %q, want us-central1", got)
	}
	if got := subnetworkRegion("projects/p/global/networks/n"); got != "" {
		t.Errorf("a non-regional link should yield no region, got %q", got)
	}
}

// A compute delete is asynchronous: the API call succeeds and the real failure only appears in
// the finished operation, so it has to be read out of the operation rather than the call.
func TestOperationError(t *testing.T) {
	if err := operationError(nil); err != nil {
		t.Errorf("a nil operation is not an error, got %v", err)
	}
	if err := operationError(&compute.Operation{Status: "DONE"}); err != nil {
		t.Errorf("an operation with no error payload is not an error, got %v", err)
	}

	op := &compute.Operation{Error: &compute.OperationError{Errors: []*compute.OperationErrorErrors{{
		Code:    "RESOURCE_IN_USE_BY_ANOTHER_RESOURCE",
		Message: "The subnetwork resource is already being used",
	}}}}
	err := operationError(op)
	if !errors.Is(err, ErrResourceInUse) {
		t.Errorf("expected ErrResourceInUse, got %v", err)
	}
	if !strings.Contains(err.Error(), "already being used") {
		t.Errorf("the GCP message should be preserved, got %q", err)
	}

	op = &compute.Operation{Error: &compute.OperationError{Errors: []*compute.OperationErrorErrors{{
		Code: "QUOTA_EXCEEDED", Message: "nope",
	}}}}
	if err := operationError(op); err == nil || errors.Is(err, ErrResourceInUse) {
		t.Errorf("an unrelated code must not map to ErrResourceInUse, got %v", err)
	}
}

func TestAnnotateOperationError(t *testing.T) {
	apiErr := &googleapi.Error{Code: 400, Errors: []googleapi.ErrorItem{{
		Reason:  "resourceInUseByAnotherResource",
		Message: "still referenced",
	}}}
	if err := annotateOperationError(apiErr); !errors.Is(err, ErrResourceInUse) {
		t.Errorf("expected ErrResourceInUse, got %v", err)
	}

	other := &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "forbidden"}}}
	if err := annotateOperationError(other); errors.Is(err, ErrResourceInUse) {
		t.Error("an unrelated reason must not map to ErrResourceInUse")
	}
	if annotateOperationError(nil) != nil {
		t.Error("nil must stay nil")
	}
}

func TestIsNotFound(t *testing.T) {
	if !isNotFound(&googleapi.Error{Code: 404}) {
		t.Error("404 should be recognised as not-found")
	}
	if isNotFound(&googleapi.Error{Code: 409}) || isNotFound(errors.New("nope")) {
		t.Error("only a 404 is not-found")
	}
}

func TestNetworkUsage(t *testing.T) {
	var empty NetworkUsage
	if empty.InUse() {
		t.Error("an empty usage is not in use")
	}
	if empty.Describe() != "" {
		t.Errorf("an empty usage describes as empty, got %q", empty.Describe())
	}

	usage := NetworkUsage{
		Instances:       []string{"vm-1", "vm-2"},
		ForwardingRules: []string{"fr-1"},
		CloudRun:        []string{"svc-1"},
	}
	if !usage.InUse() {
		t.Error("expected in use")
	}
	describe := usage.Describe()
	for _, want := range []string{"2 instance(s): vm-1, vm-2", "1 forwarding rule(s): fr-1", "1 Cloud Run service(s): svc-1"} {
		if !strings.Contains(describe, want) {
			t.Errorf("Describe() missing %q: %s", want, describe)
		}
	}

	// Any single category is enough to veto a cleanup.
	if !(NetworkUsage{CloudRun: []string{"svc"}}).InUse() {
		t.Error("a Cloud Run service alone means in use")
	}
}
