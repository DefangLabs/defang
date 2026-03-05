package tokenstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalDirTokenStore_SaveLoadDelete(t *testing.T) {
	dir := t.TempDir()
	s := &LocalDirTokenStore{Dir: dir}

	const key = "mytoken"
	const token = "secret-value"

	if err := s.Save(key, token); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load(key)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != token {
		t.Errorf("Load = %q, want %q", got, token)
	}

	if err := s.Delete(key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := s.Load(key); err == nil {
		t.Error("Load after Delete: expected error, got nil")
	}
}

func TestLocalDirTokenStore_SaveCreatesDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "subdir")
	s := &LocalDirTokenStore{Dir: dir}

	if err := s.Save("k", "v"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("directory not created: %v", err)
	}
}

func TestLocalDirTokenStore_SaveFilePermissions(t *testing.T) {
	dir := t.TempDir()
	s := &LocalDirTokenStore{Dir: dir}

	if err := s.Save("k", "v"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "k"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

func TestLocalDirTokenStore_LoadMissingKey(t *testing.T) {
	dir := t.TempDir()
	s := &LocalDirTokenStore{Dir: dir}

	if _, err := s.Load("nonexistent"); err == nil {
		t.Error("Load of missing key: expected error, got nil")
	}
}

func TestLocalDirTokenStore_DeleteMissingKey(t *testing.T) {
	dir := t.TempDir()
	s := &LocalDirTokenStore{Dir: dir}

	if err := s.Delete("nonexistent"); err != nil {
		t.Errorf("Delete of missing key: expected no error, got %v", err)
	}
}

func TestLocalDirTokenStore_EmptyDir(t *testing.T) {
	s := &LocalDirTokenStore{Dir: ""}

	if err := s.Save("k", "v"); err == nil {
		t.Error("Save with empty Dir: expected error, got nil")
	}
	if _, err := s.Load("k"); err == nil {
		t.Error("Load with empty Dir: expected error, got nil")
	}
	if err := s.Delete("k"); err == nil {
		t.Error("Delete with empty Dir: expected error, got nil")
	}
}

func TestLocalDirTokenStore_EmptyKey(t *testing.T) {
	dir := t.TempDir()
	s := &LocalDirTokenStore{Dir: dir}

	if err := s.Save("", "v"); err == nil {
		t.Error("Save with empty key: expected error, got nil")
	}
	if _, err := s.Load(""); err == nil {
		t.Error("Load with empty key: expected error, got nil")
	}
	if err := s.Delete(""); err == nil {
		t.Error("Delete with empty key: expected error, got nil")
	}
}

func TestLocalDirTokenStore_OverwriteToken(t *testing.T) {
	dir := t.TempDir()
	s := &LocalDirTokenStore{Dir: dir}

	if err := s.Save("k", "first"); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	if err := s.Save("k", "second"); err != nil {
		t.Fatalf("Save second: %v", err)
	}

	got, err := s.Load("k")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != "second" {
		t.Errorf("Load = %q, want %q", got, "second")
	}
}

func TestLocalDirTokenStore_MultipleKeys(t *testing.T) {
	dir := t.TempDir()
	s := &LocalDirTokenStore{Dir: dir}

	keys := map[string]string{
		"key1":                      "token1",
		"key2":                      "token2",
		"key3":                      "token3",
		"user1@cluster1.defang.dev": "token4with special chars !@#$%^&*()",
	}

	for k, v := range keys {
		if err := s.Save(k, v); err != nil {
			t.Fatalf("Save %q: %v", k, err)
		}
	}

	for k, want := range keys {
		got, err := s.Load(k)
		if err != nil {
			t.Fatalf("Load %q: %v", k, err)
		}
		if got != want {
			t.Errorf("Load(%q) = %q, want %q", k, got, want)
		}
	}
}
