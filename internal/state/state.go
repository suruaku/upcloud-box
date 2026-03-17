package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultPath = ".upcloud-app-platform.state.json"

type State struct {
	ServerUUID     string `json:"server_uuid"`
	PublicIP       string `json:"public_ip"`
	LastDeployedAt string `json:"last_deployed_at"`
	LastDeployMode string `json:"last_deploy_mode"`
}

func New() State {
	return State{}
}

func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state %q: %w", path, err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state %q: %w", path, err)
	}

	return &s, nil
}

func Save(path string, s State) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("state path is required")
	}

	if err := ensureParentDir(path); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state json: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write state %q: %w", path, err)
	}

	return nil
}

func (s *State) MarkDeploy(t time.Time) {
	s.LastDeployedAt = t.UTC().Format(time.RFC3339)
}

func (s *State) MarkDeployAt(t time.Time) {
	s.LastDeployedAt = t.UTC().Format(time.RFC3339)
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent dir %q: %w", dir, err)
	}
	return nil
}
