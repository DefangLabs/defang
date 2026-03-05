package tokenstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/DefangLabs/defang/src/pkg/term"
)

type TokenStore interface {
	Save(key string, token string) error
	Load(key string) (string, error)
	List(prefix string) ([]string, error)
	Delete(key string) error
}

// Backwards-compatible token store that saves tokens as files in stateDir
// TODO: consider using os provided keyring
type LocalDirTokenStore struct {
	mu  sync.RWMutex
	Dir string
}

func (s *LocalDirTokenStore) Save(key string, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tokenFile, err := s.getTokenFile(key)
	if err != nil {
		return err
	}

	term.Debug("Saving access token to", tokenFile)
	dir, _ := filepath.Split(tokenFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create token directory: %w", err)
	}
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		return fmt.Errorf("failed to save access token: %w", err)
	}
	return nil
}

func (s *LocalDirTokenStore) Load(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tokenFile, err := s.getTokenFile(key)
	if err != nil {
		return "", err
	}
	term.Debug("Reading access token from file", tokenFile)
	all, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", fmt.Errorf("failed to read token: %w", err)
	}
	return string(all), nil
}

func (s *LocalDirTokenStore) List(prefix string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Dir == "" {
		return nil, errors.New("token store directory not set")
	}
	dir, filePrefix := filepath.Split(filepath.Join(s.Dir, prefix))

	// Ensure the resolved directory is within the token store base directory to prevent path traversal
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve token directory: %w", err)
	}
	baseDir, err := filepath.Abs(s.Dir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve token store directory: %w", err)
	}
	if !strings.HasPrefix(dir, baseDir) {
		return nil, errors.New("invalid token prefix")
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no tokens if directory doesn't exist
		}
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}
	var keys []string
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), filePrefix) {
			keys = append(keys, file.Name())
		}
	}
	return keys, nil
}

func (s *LocalDirTokenStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tokenFile, err := s.getTokenFile(key)
	if err != nil {
		return err
	}
	if err := os.Remove(tokenFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to delete token: %w", err)
	}
	term.Debug("Removed token file:", tokenFile)
	return nil
}

func (s *LocalDirTokenStore) getTokenFile(key string) (string, error) {
	if s.Dir == "" {
		return "", errors.New("token store directory not set")
	}
	if key == "" {
		return "", errors.New("token store key is empty")
	}
	tokenFilePath := filepath.Join(s.Dir, key)
	absTokenFilePath, err := filepath.Abs(tokenFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve token file path: %w", err)
	}
	absDir, err := filepath.Abs(s.Dir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve token store directory: %w", err)
	}
	if !strings.HasPrefix(absTokenFilePath, absDir) {
		return "", errors.New("invalid token key")
	}
	return absTokenFilePath, nil
}
