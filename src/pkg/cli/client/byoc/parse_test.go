package byoc

import (
	"context"
	"os"
	"testing"
)

type mockObj struct {
	name string
	size int64
}

func (m *mockObj) Name() string { return m.name }
func (m *mockObj) Size() int64  { return m.size }

func TestParsePulumiStackObject(t *testing.T) {
	t.Run("Parse empty stack", func(t *testing.T) {
		obj := &mockObj{name: "prefix/teststack.json", size: 600}
		stack, state, err := ParsePulumiStackObject(t.Context(), obj, "bucket", "prefix/",
			func(ctx context.Context, bucket, object string) ([]byte, error) {
				if object != "prefix/teststack.json" {
					t.Errorf("expected object name to be loaded is %q, got %q", "prefix/teststack.json", object)
				}
				if bucket != "bucket" {
					t.Errorf("expected bucket name to be loaded is %q, got %q", "bucket", bucket)
				}
				return []byte(`{"version":3,"checkpoint":{"latest":{"resources":[]}}}`), nil
			})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if stack != "" {
			t.Fatalf("expected empty stack name, got %q", stack)
		}
		if state != nil {
			t.Fatalf("expected nil state for empty stack, got %+v", state)
		}
	})

	t.Run("Ignore non-json file", func(t *testing.T) {
		obj := &mockObj{name: "prefix/not_a_stack.txt", size: 1000}
		stack, state, err := ParsePulumiStackObject(t.Context(), obj, "bucket", "prefix/",
			func(ctx context.Context, bucket, object string) ([]byte, error) {
				t.Fatalf("expected no object to be loaded, but got load request for %q in bucket %q", object, bucket)
				return nil, nil
			})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if stack != "" {
			t.Fatalf("expected empty stack name, got %q", stack)
		}
		if state != nil {
			t.Fatalf("expected nil state for non-json file, got %+v", state)
		}
	})

	t.Run("Parse valid stack", func(t *testing.T) {
		bytes, err := os.ReadFile("testdata/valid_pulumi_stack.json")
		if err != nil {
			t.Fatalf("failed to read testdata file: %v", err)
		}
		obj := &mockObj{name: "prefix/valid_pulumi_stack.json", size: int64(len(bytes))}
		stack, state, err := ParsePulumiStackObject(t.Context(), obj, "bucket", "prefix/",
			func(ctx context.Context, bucket, object string) ([]byte, error) {
				if object != "prefix/valid_pulumi_stack.json" {
					t.Errorf("expected object name to be loaded is %q, got %q", "prefix/valid_pulumi_stack.json", object)
				}
				if bucket != "bucket" {
					t.Errorf("expected bucket name to be loaded is %q, got %q", "bucket", bucket)
				}
				return bytes, nil
			})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if stack != "valid_pulumi_stack (pending \"html-css-js.internal\") (pending \"alb\")" {
			t.Fatalf("expected stack name to be %q, got %q", "valid_stack", stack)
		}
		if state == nil {
			t.Fatalf("expected non-nil state for valid stack")
		}
		if len(state.Checkpoint.Latest.Resources) == 0 {
			t.Fatalf("expected non-empty resources in state")
		}
		if len(state.Checkpoint.Latest.PendingOperations) != 2 {
			t.Fatalf("expected 2 pending operations in state, got %d", len(state.Checkpoint.Latest.PendingOperations))
		}
	})
}
