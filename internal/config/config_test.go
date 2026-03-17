package config

import (
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
