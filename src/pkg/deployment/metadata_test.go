package deployment

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestReadState(t *testing.T) {
	metadata := func(etag, name, id string, summary bool) Metadata {
		return Metadata{Version: Version, Provider: "scaleway", Project: "demo", Stack: "prod", Etag: etag, Summary: summary,
			Services: map[string]Service{name: {Kind: "container", ID: id, Name: "unpredictable-name"}}}
	}
	resource := func(m Metadata, deleted bool) map[string]any {
		encoded, err := m.Encode()
		if err != nil {
			t.Fatal(err)
		}
		return map[string]any{"delete": deleted, "outputs": map[string]any{OutputName: encoded}}
	}
	future := metadata("new", "web", "new-id", true)
	future.Version = 99
	tests := []struct {
		name      string
		resources []map[string]any
		etag      string
		wantID    string
		wantCount int
		wantErr   bool
		missing   bool
	}{
		{name: "physical identity", resources: []map[string]any{resource(metadata("new", "web", "uuid", true), false)}, wantID: "uuid", wantCount: 1},
		{name: "partial deployment", resources: []map[string]any{resource(metadata("new", "web", "uuid", false), false)}, etag: "new", wantID: "uuid", wantCount: 1},
		{name: "replacement", resources: []map[string]any{resource(metadata("new", "web", "old-id", false), true), resource(metadata("new", "web", "new-id", false), false)}, etag: "new", wantID: "new-id", wantCount: 1},
		{name: "select attempt", resources: []map[string]any{resource(metadata("old", "web", "old-id", true), false), resource(metadata("new", "web", "new-id", false), false)}, etag: "new", wantID: "new-id", wantCount: 1},
		{name: "summary excludes removed services", resources: []map[string]any{resource(metadata("old", "removed", "old-id", false), false), resource(metadata("new", "web", "new-id", true), false)}, wantID: "new-id", wantCount: 1},
		{name: "ambiguous attempts", resources: []map[string]any{resource(metadata("old", "web", "old-id", false), false), resource(metadata("new", "other", "new-id", false), false)}, wantErr: true},
		{name: "unknown output", resources: []map[string]any{{"outputs": map[string]any{OutputName: "04da6b54-80e4-46f7-96ec-b56ff0331ba9"}}}, missing: true},
		{name: "secret output stays opaque", resources: []map[string]any{{"outputs": map[string]any{OutputName: map[string]any{"ciphertext": "secret"}}}}, missing: true},
		{name: "legacy deployment", missing: true},
		{name: "future contract", resources: []map[string]any{resource(future, false)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, checkpoint := range []bool{true, false} {
				snapshot := map[string]any{"resources": tt.resources}
				state := map[string]any{"version": 3, "deployment": snapshot}
				if checkpoint {
					state = map[string]any{"version": 3, "checkpoint": map[string]any{"latest": snapshot}}
				}
				data, err := json.Marshal(state)
				if err != nil {
					t.Fatal(err)
				}
				got, err := ReadState(data, "scaleway", "demo", "prod", tt.etag)
				if tt.missing {
					if !errors.Is(err, ErrNotFound) {
						t.Fatalf("want missing, got %v", err)
					}
					continue
				}
				if tt.wantErr {
					if err == nil {
						t.Fatal("expected error")
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				if got.Services["web"].ID != tt.wantID || len(got.Services) != tt.wantCount {
					t.Fatalf("unexpected metadata: %+v", got)
				}
				_, err = ReadState(data, "scaleway", "other-project", "prod", "")
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("cross-project metadata leaked: %v", err)
				}
			}
		})
	}
}
