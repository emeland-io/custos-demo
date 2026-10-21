package internal

import (
	"encoding/json"
	"os"
)

const stateFilePath = ".devcluster-state.json"

type State struct {
	BaseURL           string `json:"base_url"`
	DexIssuer         string `json:"dex_issuer"`
	DexLocalAddr      string `json:"dex_local_addr"`
	PortForwardPID    int    `json:"port_forward_pid"`
	DexPortForwardPID int    `json:"dex_port_forward_pid"`
}

func SaveState(s State) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFilePath, data, 0o600)
}

func LoadState() (State, error) {
	var s State
	data, err := os.ReadFile(stateFilePath)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

func ClearState() error {
	err := os.Remove(stateFilePath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
