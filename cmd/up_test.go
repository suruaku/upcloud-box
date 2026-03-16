package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrBootstrapConfigForUpCreatesConfigOnly(t *testing.T) {
	workDir := t.TempDir()
	t.Chdir(workDir)

	configPath := filepath.Join(workDir, "upcloud-app-platform.yaml")

	cfg, bootstrap, err := loadOrBootstrapConfigForUp(configPath)
	if err != nil {
		t.Fatalf("load or bootstrap config: %v", err)
	}
	if cfg == nil {
		t.Fatal("config is nil")
	}
	if !bootstrap.ConfigCreated {
		t.Fatal("expected config to be created")
	}

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "cloud-init.yaml")); !os.IsNotExist(err) {
		t.Fatalf("cloud-init should not be created by default; err=%v", err)
	}
}

func TestLoadOrBootstrapConfigForUpUsesExistingConfig(t *testing.T) {
	workDir := t.TempDir()
	t.Chdir(workDir)

	configPath := filepath.Join(workDir, "upcloud-app-platform.yaml")
	if err := writeConfig(configPath, false, ""); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, bootstrap, err := loadOrBootstrapConfigForUp(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if bootstrap.ConfigCreated {
		t.Fatal("did not expect config bootstrap")
	}
}
