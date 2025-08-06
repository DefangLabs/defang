//go:integration

package aci

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/http"
)

func TestCreateUploadURL(t *testing.T) {
	// Create a new container instance
	container := NewContainerInstance("defang-cd", "westeurope")

	// Call the CreateUploadURL method
	url, err := container.CreateUploadURL(context.Background(), "sha256-Jv4+base64/encoded_digest=")
	if err != nil {
		t.Fatalf("failed to create upload URL: %v", err)
	}

	// Verify the URL is not empty
	if url == "" {
		t.Fatal("expected non-empty upload URL")
	}

	t.Logf("Upload URL: %s", url)
	header := http.Header{"Content-Type": []string{"application/text"}}
	header.Set("X-Ms-Blob-Type", "BlockBlob")
	resp, err := http.PutWithHeader(context.Background(), url, header, strings.NewReader("test content"))
	if err != nil {
		t.Fatalf("failed to upload content: %v", err)
	}
	defer resp.Body.Close()

	// Verify the response
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status OK, got %v: %s", resp.Status, body)
	}
}
