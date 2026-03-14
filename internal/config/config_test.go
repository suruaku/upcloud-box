package config

import "testing"

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

func TestDeriveHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple", input: "my-app", want: "my-app-prod"},
		{name: "spaces and uppercase", input: "My App", want: "my-app-prod"},
		{name: "symbols", input: "@@@", want: "app-prod"},
		{name: "empty", input: "", want: "app-prod"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := deriveHostname(tc.input)
			if got != tc.want {
				t.Fatalf("deriveHostname(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
