package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ikox01/upcloud-box/internal/config"
	"github.com/ikox01/upcloud-box/internal/infra/factory"
	sshrunner "github.com/ikox01/upcloud-box/internal/ssh"
	"github.com/ikox01/upcloud-box/internal/state"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show infrastructure and app status",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := state.Load(state.DefaultPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fmt.Printf("No state file found at %s\n", state.DefaultPath)
				return nil
			}
			return err
		}

		fmt.Printf("State file: %s\n", state.DefaultPath)
		fmt.Printf("server_uuid: %s\n", renderOrDash(s.ServerUUID))
		fmt.Printf("public_ip: %s\n", renderOrDash(s.PublicIP))
		fmt.Printf("last_successful_image: %s\n", renderOrDash(s.LastSuccessfulImage))
		fmt.Printf("last_deployed_at: %s\n", renderOrDash(s.LastDeployedAt))

		if strings.TrimSpace(s.ServerUUID) == "" {
			fmt.Println("Remote infra: none tracked")
			return nil
		}

		provider, err := factory.NewDefaultProvider()
		if err != nil {
			fmt.Printf("Remote infra: skipped (%v)\n", err)
			return nil
		}

		serverInfo, err := provider.Get(context.Background(), s.ServerUUID)
		if err != nil {
			if isLikelyNotFound(err) {
				fmt.Printf("Remote infra: server %s not found\n", s.ServerUUID)
				return nil
			}
			return err
		}

		fmt.Printf("Remote infra: %s (%s)\n", serverInfo.ServerID, serverInfo.Hostname)
		fmt.Printf("remote_state: %s\n", renderOrDash(serverInfo.State))
		fmt.Printf("remote_public_ipv4: %s\n", renderOrDash(serverInfo.PublicIPv4))

		renderRemoteAppSummary(s, serverInfo.PublicIPv4)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func renderOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}

func isLikelyNotFound(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "status code 404") || strings.Contains(msg, " 404")
}

func renderRemoteAppSummary(s *state.State, remoteIPv4 string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Printf("Remote app: skipped (load config %q: %v)\n", cfgFile, err)
		return
	}

	host := strings.TrimSpace(s.PublicIP)
	if host == "" {
		host = strings.TrimSpace(remoteIPv4)
	}
	if host == "" {
		fmt.Println("Remote app: skipped (no public IP available)")
		return
	}

	runner, err := sshrunner.NewRunner(sshrunner.Config{
		User:           cfg.SSH.User,
		PrivateKeyPath: cfg.SSH.PrivateKeyPath,
		ConfigDir:      filepath.Dir(cfgFile),
		ConnectTimeout: time.Duration(cfg.SSH.ConnectTimeoutSeconds) * time.Second,
		RetryInterval:  3 * time.Second,
	})
	if err != nil {
		fmt.Printf("Remote app: skipped (%v)\n", err)
		return
	}

	containerName := strings.TrimSpace(cfg.Deploy.ContainerName)
	if containerName == "" {
		fmt.Println("Remote app: skipped (deploy.container_name is empty)")
		return
	}

	result, err := runner.Run(context.Background(), host, fmt.Sprintf("docker ps -a --filter name=^/%s$ --format '{{.Names}}|{{.Status}}|{{.Image}}'", shellQuote(containerName)))
	if err != nil {
		fmt.Printf("Remote app: unavailable (%v)\n", err)
		return
	}

	line := strings.TrimSpace(result.Stdout)
	if line == "" {
		fmt.Printf("Remote app: container %q not found\n", containerName)
		return
	}

	parts := strings.SplitN(line, "|", 3)
	containerStatus := "-"
	containerImage := "-"
	if len(parts) > 1 {
		containerStatus = renderOrDash(parts[1])
	}
	if len(parts) > 2 {
		containerImage = renderOrDash(parts[2])
	}

	healthStatus := "unhealthy"
	healthErr := ""
	if _, err := runner.Run(context.Background(), host, fmt.Sprintf("curl -fsS --max-time 5 %s >/dev/null", shellQuote(cfg.Deploy.HealthcheckURL))); err == nil {
		healthStatus = "healthy"
	} else {
		healthErr = err.Error()
	}

	fmt.Printf("Remote app: %s\n", renderOrDash(containerName))
	fmt.Printf("container_status: %s\n", containerStatus)
	fmt.Printf("container_image: %s\n", containerImage)
	fmt.Printf("health_url: %s\n", renderOrDash(cfg.Deploy.HealthcheckURL))
	fmt.Printf("health: %s\n", healthStatus)
	if healthErr != "" {
		fmt.Printf("health_error: %s\n", healthErr)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(strings.TrimSpace(s), "'", "'\"'\"'") + "'"
}
