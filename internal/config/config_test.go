package config

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestValidateAllowsEmptySSHPrivateKeyPath(t *testing.T) {
	t.Parallel()

	cfg := Default()
	cfg.SSH.PrivateKeyPath = ""
	cfg.Provision.CloudInitPath = ""
	cfg.Provision.Hostname = ""

	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate config: %v", err)
	}
}

func TestDeriveHostnameWithSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple", input: "my-app", want: "my-app-1a2b3c4d"},
		{name: "spaces and uppercase", input: "My App", want: "my-app-1a2b3c4d"},
		{name: "symbols", input: "@@@", want: "app-1a2b3c4d"},
		{name: "empty", input: "", want: "app-1a2b3c4d"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := deriveHostnameWithSuffix(tc.input, "1a2b3c4d")
			if got != tc.want {
				t.Fatalf("deriveHostnameWithSuffix(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDeriveHostnameIncludesShortHexSuffix(t *testing.T) {
	t.Parallel()

	got := deriveHostname("my-app")
	pattern := regexp.MustCompile(`^my-app-[a-f0-9]{8}$`)
	if !pattern.MatchString(got) {
		t.Fatalf("deriveHostname() = %q, want my-app-<8 hex chars>", got)
	}
}

func TestLoadMigratesLegacyDerivedHostname(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	configPath := filepath.Join(workDir, "upcloud-app-platform.yaml")
	configYAML := "project: upcloud-app-platform\n" +
		"upcloud:\n  zone: fi-hel1\n  plan: 1xCPU-2GB\n  template: Ubuntu Server 24.04 LTS\n" +
		"provision:\n  hostname: upcloud-app-platform-prod\n" +
		"ssh:\n  user: ubuntu\n  private_key_path: \"\"\n  connect_timeout_seconds: 120\n" +
		"deploy:\n  container_name: my-app\n  image: nginxdemos/hello:latest\n  port: 80:80\n  env_file: .env.prod\n  healthcheck_url: http://localhost/\n  healthcheck_timeout_seconds: 60\n  healthcheck_interval_seconds: 3\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	pattern := regexp.MustCompile(`^upcloud-app-platform-[a-f0-9]{8}$`)
	if !pattern.MatchString(cfg.Provision.Hostname) {
		t.Fatalf("hostname = %q, want upcloud-app-platform-<8 hex chars>", cfg.Provision.Hostname)
	}
}

func TestLoadKeepsCustomHostname(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	configPath := filepath.Join(workDir, "upcloud-app-platform.yaml")
	configYAML := "project: upcloud-app-platform\n" +
		"upcloud:\n  zone: fi-hel1\n  plan: 1xCPU-2GB\n  template: Ubuntu Server 24.04 LTS\n" +
		"provision:\n  hostname: custom-hostname\n" +
		"ssh:\n  user: ubuntu\n  private_key_path: \"\"\n  connect_timeout_seconds: 120\n" +
		"deploy:\n  container_name: my-app\n  image: nginxdemos/hello:latest\n  port: 80:80\n  env_file: .env.prod\n  healthcheck_url: http://localhost/\n  healthcheck_timeout_seconds: 60\n  healthcheck_interval_seconds: 3\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Provision.Hostname != "custom-hostname" {
		t.Fatalf("hostname = %q, want custom-hostname", cfg.Provision.Hostname)
	}
}
