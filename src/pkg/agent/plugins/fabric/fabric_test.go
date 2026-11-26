package fabric

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/openai/openai-go/option"
)

func TestDefaultModelOnWire(t *testing.T) {
	var gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		gotModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	gk := genkit.Init(ctx,
		genkit.WithDefaultModel(provider+"/default"),
		genkit.WithPlugins(&OpenAI{APIKey: "token", Opts: []option.RequestOption{option.WithBaseURL(srv.URL)}}),
	)
	_, err := genkit.Generate(ctx, gk, ai.WithPrompt("hi"),
		ai.WithStreaming(func(context.Context, *ai.ModelResponseChunk) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if gotModel != "default" {
		t.Errorf("model sent to Fabric = %q, want %q", gotModel, "default")
	}
}
