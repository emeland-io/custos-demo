package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const stateFileName = ".devcluster-state.json"

type State struct {
	BaseURL           string `json:"base_url"`
	DexIssuer         string `json:"dex_issuer"`
	DexLocalAddr      string `json:"dex_local_addr"`
	PortForwardPID    int    `json:"port_forward_pid"`
	DexPortForwardPID int    `json:"dex_port_forward_pid"`
}

// stateFilePath anchors the state file to the repo root rather than
// the calling process's cwd: this plan's own workflow runs `up` from
// the repo root but `baseurl`/`dexaddr` from frontend/ (see Task 9's
// Playwright run), so a cwd-relative path would silently miss what
// `up` wrote.
func stateFilePath() (string, error) {
	root, err := RepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, stateFileName), nil
}

func SaveState(s State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path, err := stateFilePath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func LoadState() (State, error) {
	var s State
	path, err := stateFilePath()
	if err != nil {
		return s, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

func ClearState() error {
	path, err := stateFilePath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
