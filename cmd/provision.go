package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/suruaku/upcloud-app-platform/internal/config"
	"github.com/suruaku/upcloud-app-platform/internal/infra"
	"github.com/suruaku/upcloud-app-platform/internal/infra/factory"
	sshrunner "github.com/suruaku/upcloud-app-platform/internal/ssh"
	"github.com/suruaku/upcloud-app-platform/internal/state"
)

var provisionWaitTimeout time.Duration

var provisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Provision secure Docker host on UpCloud",
	RunE: func(cmd *cobra.Command, args []string) error {
		logVerbose("starting provision with config=%s wait-timeout=%s", cfgFile, provisionWaitTimeout)
		cfg, err := loadConfigOrErr()
		if err != nil {
			return err
		}

		cloudInitRaw, err := resolveCloudInitRaw(cfg, cfgFile)
		if err != nil {
			return wrapUserError("read cloud-init", err)
		}

		s, err := loadOrInitState(state.DefaultPath)
		if err != nil {
			return wrapUserError("load state", err)
		}
		if strings.TrimSpace(s.ServerUUID) != "" {
			return wrapUserError("validate state", fmt.Errorf("state already has server_uuid %q; destroy it first or clear %s", s.ServerUUID, state.DefaultPath))
		}

		provider, err := factory.NewDefaultProvider()
		if err != nil {
			return wrapUserError("initialize provider", err)
		}

		var result infra.ProvisionResult
		if err := runStep("Provisioning server on UpCloud...", "Server provisioning request accepted", func() error {
			var stepErr error
			result, stepErr = provider.Provision(context.Background(), infra.ProvisionRequest{
				Zone:         cfg.UpCloud.Zone,
				Plan:         cfg.UpCloud.Plan,
				Template:     cfg.UpCloud.Template,
				Hostname:     cfg.Provision.Hostname,
				CloudInitRaw: cloudInitRaw,
			})
			return stepErr
		}); err != nil {
			return wrapUserError("provision server", err)
		}

		s.ServerUUID = result.ServerID
		s.PublicIP = ""
		if err := state.Save(state.DefaultPath, *s); err != nil {
			return wrapUserError("save state", err)
		}

		var serverInfo infra.ServerInfo
		if err := runStep("Waiting for server to become ready...", "Server is started", func() error {
			var stepErr error
			serverInfo, stepErr = provider.WaitReady(context.Background(), result.ServerID, provisionWaitTimeout)
			return stepErr
		}); err != nil {
			return wrapUserError("wait for server readiness", err)
		}

		s.PublicIP = serverInfo.PublicIPv4
		if err := state.Save(state.DefaultPath, *s); err != nil {
			return wrapUserError("save state", err)
		}

		if err := runStep("Running post-provision SSH and Docker checks...", "Post-provision checks passed", func() error {
			return runPostProvisionChecks(cfg, serverInfo.PublicIPv4)
		}); err != nil {
			return wrapUserError("post-provision checks", err)
		}

		fmt.Printf("Provisioned server %s (%s)\n", serverInfo.ServerID, serverInfo.Hostname)
		fmt.Printf("State: %s\n", serverInfo.State)
		if serverInfo.PublicIPv4 != "" {
			fmt.Printf("Public IPv4: %s\n", serverInfo.PublicIPv4)
		}
		fmt.Printf("State saved to %s\n", state.DefaultPath)
		return nil
	},
}

func init() {
	provisionCmd.Flags().DurationVar(&provisionWaitTimeout, "wait-timeout", 10*time.Minute, "max time to wait for server to become started")
	rootCmd.AddCommand(provisionCmd)
}

func readCloudInitPassThrough(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cloud-init file %q: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("cloud-init file %q is empty", path)
	}
	return data, nil
}

func resolveCloudInitRaw(cfg *config.Config, cfgPath string) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	cloudInitPath := strings.TrimSpace(cfg.Provision.CloudInitPath)
	if cloudInitPath != "" {
		return readCloudInitPassThrough(cloudInitPath)
	}

	keys, err := resolveSSHAuthorizedKeys(nil, cfgPath)
	if err != nil {
		return nil, err
	}

	return buildCloudInit(cfg.SSH.User, keys), nil
}

func loadOrInitState(path string) (*state.State, error) {
	s, err := state.Load(path)
	if err == nil {
		return s, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		newState := state.New()
		if saveErr := state.Save(path, newState); saveErr != nil {
			return nil, saveErr
		}
		return &newState, nil
	}
	return nil, err
}

func runPostProvisionChecks(cfg *config.Config, host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("post-provision checks require a public IPv4 address")
	}

	runner, err := sshrunner.NewRunner(sshrunner.Config{
		User:           cfg.SSH.User,
		PrivateKeyPath: cfg.SSH.PrivateKeyPath,
		ConfigDir:      filepath.Dir(cfgFile),
		ConnectTimeout: time.Duration(cfg.SSH.ConnectTimeoutSeconds) * time.Second,
		RetryInterval:  3 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create ssh runner: %w", err)
	}

	const checkTimeout = 5 * time.Minute

	if _, err := runner.RunWithRetry(context.Background(), host, "true", checkTimeout); err != nil {
		return fmt.Errorf("post-provision ssh connectivity check failed: %w", err)
	}

	if _, err := runner.RunWithRetry(context.Background(), host, "docker info >/dev/null 2>&1", checkTimeout); err != nil {
		return fmt.Errorf("post-provision docker check failed: %w", err)
	}

	return nil
}
