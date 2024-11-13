package command

import (
	"encoding/json"
	"path/filepath"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
)

const DEFANG_LOCAL_STATE_FILENAME = ".defang"

type LocalState struct {
	WorkingDir string               `json:"-"`
	Provider   cliClient.ProviderID `json:"provider"`
}

func (state *LocalState) Read() error {
	bytes, err := ReadHiddenFile(state.StateFilePath())
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, state)
}

func (state *LocalState) Write() error {
	if bytes, err := json.MarshalIndent(state, "", "  "); err != nil {
		return err
	} else {
		return WriteHiddenFile(state.StateFilePath(), bytes, 0644)
	}
}

func (state *LocalState) StateFilePath() string {
	return filepath.Join(state.WorkingDir, DEFANG_LOCAL_STATE_FILENAME)
}
