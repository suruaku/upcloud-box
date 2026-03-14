package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ikox01/upcloud-box/internal/config"
)

func TestResolveCloudInitRawUsesConfigFileWhenPathSet(t *testing.T) {
	workDir := t.TempDir()
	cloudInitPath := filepath.Join(workDir, "cloud-init.yaml")
	want := []byte("#cloud-config\nusers: []\n")
	if err := os.WriteFile(cloudInitPath, want, 0o600); err != nil {
		t.Fatalf("write cloud-init: %v", err)
	}

	cfg := config.Default()
	cfg.Provision.CloudInitPath = cloudInitPath

	got, err := resolveCloudInitRaw(&cfg, filepath.Join(workDir, "upcloud-box.yaml"))
	if err != nil {
		t.Fatalf("resolve cloud-init raw: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("cloud-init mismatch\nwant:\n%s\n\ngot:\n%s", string(want), string(got))
	}
}

func TestResolveCloudInitRawFailsWithoutPublicKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatalf("mkdir .ssh: %v", err)
	}

	cfg := config.Default()
	cfg.Provision.CloudInitPath = ""

	_, err := resolveCloudInitRaw(&cfg, filepath.Join(home, "upcloud-box.yaml"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no SSH public key found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
