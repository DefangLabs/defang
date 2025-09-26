package github

import (
	"os"
	"testing"
)

func TestGetIdToken(t *testing.T) {
	requestUrl := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	if requestUrl == "" {
		t.Skip("ACTIONS_ID_TOKEN_REQUEST_URL not set")
	}

	jwt, err := GetIdToken(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if jwt == "" {
		t.Error("empty jwt")
	}
}
