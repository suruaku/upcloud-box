package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectDeployMode(t *testing.T) {
	t.Parallel()

	t.Run("detects docker-compose yaml", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		configPath := filepath.Join(dir, "upcloud-app-platform.yaml")
		if err := os.WriteFile(configPath, []byte("project: test\n"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "docker-compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
			t.Fatalf("write compose: %v", err)
		}

		mode, composePath, composeFileName, err := detectDeployMode(configPath)
		if err != nil {
			t.Fatalf("detect mode: %v", err)
		}
		if mode != deployModeCompose {
			t.Fatalf("mode = %q, want %q", mode, deployModeCompose)
		}
		if composeFileName != "docker-compose.yaml" {
			t.Fatalf("compose file name = %q, want docker-compose.yaml", composeFileName)
		}
		if composePath != filepath.Join(dir, "docker-compose.yaml") {
			t.Fatalf("compose path = %q, want %q", composePath, filepath.Join(dir, "docker-compose.yaml"))
		}
	})

	t.Run("falls back to compose yaml", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		configPath := filepath.Join(dir, "upcloud-app-platform.yaml")
		if err := os.WriteFile(configPath, []byte("project: test\n"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o600); err != nil {
			t.Fatalf("write compose: %v", err)
		}

		mode, composePath, composeFileName, err := detectDeployMode(configPath)
		if err != nil {
			t.Fatalf("detect mode: %v", err)
		}
		if mode != deployModeCompose {
			t.Fatalf("mode = %q, want %q", mode, deployModeCompose)
		}
		if composeFileName != "compose.yaml" {
			t.Fatalf("compose file name = %q, want compose.yaml", composeFileName)
		}
		if composePath != filepath.Join(dir, "compose.yaml") {
			t.Fatalf("compose path = %q, want %q", composePath, filepath.Join(dir, "compose.yaml"))
		}
	})

	t.Run("single mode when no compose file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		configPath := filepath.Join(dir, "upcloud-app-platform.yaml")
		if err := os.WriteFile(configPath, []byte("project: test\n"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		mode, composePath, composeFileName, err := detectDeployMode(configPath)
		if err != nil {
			t.Fatalf("detect mode: %v", err)
		}
		if mode != deployModeSingle {
			t.Fatalf("mode = %q, want %q", mode, deployModeSingle)
		}
		if composePath != "" {
			t.Fatalf("compose path = %q, want empty", composePath)
		}
		if composeFileName != "" {
			t.Fatalf("compose file name = %q, want empty", composeFileName)
		}
	})
}

func TestRemoteProjectName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "empty", input: "", expect: "app"},
		{name: "simple", input: "my-app", expect: "my-app"},
		{name: "spaces", input: "my app", expect: "my-app"},
		{name: "symbols", input: "@@@", expect: "app"},
		{name: "trim punctuation", input: "..my-app..", expect: "my-app"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := remoteProjectName(tc.input)
			if got != tc.expect {
				t.Fatalf("remoteProjectName(%q) = %q, want %q", tc.input, got, tc.expect)
			}
		})
	}
}
