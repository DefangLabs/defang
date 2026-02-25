package tokenstore

import (
	"errors"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg/term"
)

type TokenStore interface {
	Save(key string, token string) error
	Load(key string) (string, error)
	Delete(key string) error
}

// Backwards-compatible token store that saves tokens as files in stateDir
// TODO: consider using os provided keyring
type LocalDirTokenStore struct {
	Dir string
}

func (s *LocalDirTokenStore) Save(key string, token string) error {
	tokenFile, err := s.getTokenFile(key)
	if err != nil {
		return err
	}

	term.Debug("Saving access token to", tokenFile)
	os.MkdirAll(s.Dir, 0700)
	if err := os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		return fmt.Errorf("failed to save access token: %w", err)
	}
	return nil
}

func (s *LocalDirTokenStore) Load(key string) (string, error) {
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

func (s *LocalDirTokenStore) Delete(key string) error {
	tokenFile, err := s.getTokenFile(key)
	if err != nil {
		return err
	}
	if err := os.Remove(tokenFile); err != nil {
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
	return fmt.Sprintf("%s/%s", s.Dir, key), nil
}
