package cmd

import (
	"context"
	"errors"
	"fmt"
	"math"
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

var statsForNerds bool

const statusRemoteCommandTimeout = 12 * time.Second

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
		if statsForNerds {
			fmt.Printf("server_uuid: %s\n", renderOrDash(s.ServerUUID))
			fmt.Printf("public_ip: %s\n", renderOrDash(s.PublicIP))
			fmt.Printf("last_successful_image: %s\n", renderOrDash(s.LastSuccessfulImage))
			fmt.Printf("last_deployed_at: %s\n", renderOrDash(s.LastDeployedAt))
			fmt.Printf("last_deploy_mode: %s\n", renderOrDash(s.LastDeployMode))
		} else {
			fmt.Printf("Tracked server: %s\n", renderOrDash(s.ServerUUID))
		}

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

		cfg, cfgErr := config.Load(cfgFile)

		renderRemoteInfrastructureSummary(provider, cfg, cfgErr, s, serverInfo)
		renderRemoteRuntimeSummary(s, serverInfo.PublicIPv4)
		renderRemoteAppSummary(s, serverInfo.PublicIPv4)
		return nil
	},
}

func init() {
	statusCmd.Flags().BoolVar(&statsForNerds, "stats-for-nerds", false, "show detailed infrastructure and runtime diagnostics")
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

func renderRemoteInfrastructureSummary(provider infra.Provider, cfg *config.Config, cfgErr error, s *state.State, serverInfo infra.ServerInfo) {
	zone := strings.TrimSpace(serverInfo.Zone)
	if zone == "" && cfgErr == nil {
		zone = strings.TrimSpace(cfg.UpCloud.Zone)
	}

	plan := strings.TrimSpace(serverInfo.Plan)
	if plan == "" && cfgErr == nil {
		plan = strings.TrimSpace(cfg.UpCloud.Plan)
	}

	costLine := "unavailable"
	costDetails := ""
	if zone != "" && plan != "" {
		cost, err := provider.EstimateServerCost(context.Background(), zone, plan)
		if err == nil {
			costLine = fmt.Sprintf("%s%.4f/hr (~%s%.2f/mo)", currencySymbol(cost.Currency), cost.Hourly, currencySymbol(cost.Currency), cost.Monthly)
			if statsForNerds {
				costDetails = fmt.Sprintf("cost_currency: %s\ncost_hourly: %.6f\ncost_monthly: %.6f\ncost_source: %s", renderOrDash(cost.Currency), cost.Hourly, cost.Monthly, renderOrDash(cost.Source))
			}
		} else if statsForNerds {
			costDetails = fmt.Sprintf("cost_error: %s", err.Error())
		}
	}

	if !statsForNerds {
		fmt.Println("Remote infra:")
		fmt.Printf("  Server: %s (%s)\n", renderOrDash(serverInfo.ServerID), renderOrDash(serverInfo.Hostname))
		fmt.Printf("  State: %s\n", renderOrDash(serverInfo.State))
		fmt.Printf("  Public IPv4: %s\n", renderOrDash(serverInfo.PublicIPv4))
		fmt.Printf("  Plan: %s", renderOrDash(plan))
		if serverInfo.CoreCount > 0 || serverInfo.MemoryMB > 0 {
			fmt.Printf(" (%d vCPU, %s RAM)", serverInfo.CoreCount, formatMemory(serverInfo.MemoryMB))
		}
		fmt.Println()
		fmt.Printf("  Estimated cost: %s\n", costLine)
		return
	}

	fmt.Printf("Remote infra: %s (%s)\n", renderOrDash(serverInfo.ServerID), renderOrDash(serverInfo.Hostname))
	fmt.Printf("remote_state: %s\n", renderOrDash(serverInfo.State))
	fmt.Printf("remote_public_ipv4: %s\n", renderOrDash(serverInfo.PublicIPv4))
	fmt.Printf("remote_zone: %s\n", renderOrDash(zone))
	fmt.Printf("remote_plan: %s\n", renderOrDash(plan))
	fmt.Printf("remote_cores: %s\n", renderOrDash(intToString(serverInfo.CoreCount)))
	fmt.Printf("remote_memory_mb: %s\n", renderOrDash(intToString(serverInfo.MemoryMB)))
	fmt.Printf("estimated_cost: %s\n", costLine)
	if costDetails != "" {
		fmt.Println(costDetails)
	}
	if cfgErr != nil {
		fmt.Printf("config_load_error: %s\n", cfgErr.Error())
	}
	fmt.Printf("tracked_public_ip: %s\n", renderOrDash(s.PublicIP))
}

type remoteRuntimeStats struct {
	CPUUsage    string
	RAMUsage    string
	Uptime      string
	LoadAvg     string
	RootDisk    string
	CPUErr      string
	RAMErr      string
	UptimeErr   string
	LoadAvgErr  string
	RootDiskErr string
}

func renderRemoteRuntimeSummary(s *state.State, remoteIPv4 string) {
	runner, host, err := resolveRemoteRunnerAndHost(s, remoteIPv4)
	if err != nil {
		fmt.Printf("Runtime: skipped (%v)\n", err)
		return
	}

	stats := collectRemoteRuntimeStats(runner, host)
	if !statsForNerds {
		fmt.Println("Runtime:")
		fmt.Printf("  Uptime: %s\n", renderOrDash(stats.Uptime))
		fmt.Printf("  CPU usage: %s\n", renderOrDash(stats.CPUUsage))
		fmt.Printf("  RAM usage: %s\n", renderOrDash(stats.RAMUsage))
		return
	}

	fmt.Println("Runtime:")
	fmt.Printf("uptime: %s\n", renderOrDash(stats.Uptime))
	fmt.Printf("cpu_usage: %s\n", renderOrDash(stats.CPUUsage))
	fmt.Printf("ram_usage: %s\n", renderOrDash(stats.RAMUsage))
	fmt.Printf("load_avg: %s\n", renderOrDash(stats.LoadAvg))
	fmt.Printf("root_disk_usage: %s\n", renderOrDash(stats.RootDisk))
	if stats.CPUErr != "" {
		fmt.Printf("cpu_usage_error: %s\n", stats.CPUErr)
	}
	if stats.RAMErr != "" {
		fmt.Printf("ram_usage_error: %s\n", stats.RAMErr)
	}
	if stats.UptimeErr != "" {
		fmt.Printf("uptime_error: %s\n", stats.UptimeErr)
	}
	if stats.LoadAvgErr != "" {
		fmt.Printf("load_avg_error: %s\n", stats.LoadAvgErr)
	}
	if stats.RootDiskErr != "" {
		fmt.Printf("root_disk_usage_error: %s\n", stats.RootDiskErr)
	}
}

func collectRemoteRuntimeStats(runner *sshrunner.Runner, host string) remoteRuntimeStats {
	stats := remoteRuntimeStats{
		CPUUsage: "unavailable",
		RAMUsage: "unavailable",
		Uptime:   "unavailable",
		LoadAvg:  "unavailable",
		RootDisk: "unavailable",
	}

	cpu, err := runRemoteScript(runner, host, `LC_ALL=C top -bn1 2>/dev/null | awk -F',' '/^%Cpu\(s\):/ {for(i=1;i<=NF;i++){if($i ~ / id/){gsub(/[^0-9.]/,"",$i); if($i!=""){printf "%.1f%%", 100-$i; exit}}}}'`)
	if err != nil {
		stats.CPUErr = err.Error()
	} else if strings.TrimSpace(cpu) != "" {
		stats.CPUUsage = cpu
	}

	ram, err := runRemoteScript(runner, host, `if command -v free >/dev/null 2>&1; then free -m | awk '/^Mem:/ {if($2>0){printf "%dMB / %dMB (%.1f%%)", $3, $2, ($3/$2)*100}}'; fi`)
	if err != nil {
		stats.RAMErr = err.Error()
	} else if strings.TrimSpace(ram) != "" {
		stats.RAMUsage = ram
	}

	uptime, err := runRemoteScript(runner, host, `if uptime -p >/dev/null 2>&1; then uptime -p | cut -d' ' -f2-; elif [ -r /proc/uptime ]; then awk '{printf "%dh %dm", int($1/3600), int(($1%3600)/60)}' /proc/uptime; fi`)
	if err != nil {
		stats.UptimeErr = err.Error()
	} else if strings.TrimSpace(uptime) != "" {
		stats.Uptime = uptime
	}

	loadAvg, err := runRemoteScript(runner, host, `if [ -r /proc/loadavg ]; then awk '{print $1" "$2" "$3}' /proc/loadavg; else uptime | awk -F'load average: ' 'NF > 1 {print $2}'; fi`)
	if err != nil {
		stats.LoadAvgErr = err.Error()
	} else if strings.TrimSpace(loadAvg) != "" {
		stats.LoadAvg = loadAvg
	}

	rootDisk, err := runRemoteScript(runner, host, `df -h / 2>/dev/null | awk 'NR==2 {print $3" / "$2" ("$5")"}'`)
	if err != nil {
		stats.RootDiskErr = err.Error()
	} else if strings.TrimSpace(rootDisk) != "" {
		stats.RootDisk = rootDisk
	}

	return stats
}

func renderRemoteAppSummary(s *state.State, remoteIPv4 string) {
	runner, host, err := resolveRemoteRunnerAndHost(s, remoteIPv4)
	if err != nil {
		fmt.Printf("Remote app: skipped (%v)\n", err)
		return
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Printf("Remote app: skipped (load config %q: %v)\n", cfgFile, err)
		return
	}

	containerName := strings.TrimSpace(cfg.Deploy.ContainerName)
	if strings.TrimSpace(s.LastDeployMode) == string(deployModeCompose) {
		renderRemoteComposeSummary(runner, host, cfg)
		return
	}

	if containerName == "" {
		fmt.Println("Remote app: skipped (deploy.container_name is empty)")
		return
	}

	result, err := runRemoteCommand(runner, host, fmt.Sprintf("docker ps -a --filter name=^/%s$ --format '{{.Names}}|{{.Status}}|{{.Image}}'", shellQuote(containerName)))
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
	if _, err := runRemoteCommand(runner, host, fmt.Sprintf("curl -fsS --max-time 5 %s >/dev/null", shellQuote(cfg.Deploy.HealthcheckURL))); err == nil {
		healthStatus = "healthy"
	} else {
		healthErr = err.Error()
	}

	containerStats, containerStatsErr := runRemoteScript(runner, host, fmt.Sprintf("docker stats --no-stream --format '{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}' %s", shellQuote(containerName)))
	containerRuntime := "unavailable"
	if strings.TrimSpace(containerStats) != "" {
		statParts := strings.SplitN(strings.TrimSpace(containerStats), "|", 3)
		if len(statParts) == 3 {
			containerRuntime = fmt.Sprintf("CPU %s, RAM %s", renderOrDash(statParts[1]), renderOrDash(statParts[2]))
		}
	}

	if !statsForNerds {
		fmt.Println("Remote app:")
		fmt.Printf("  Name: %s\n", renderOrDash(containerName))
		fmt.Printf("  Status: %s\n", containerStatus)
		fmt.Printf("  Health: %s\n", healthStatus)
		fmt.Printf("  Container usage: %s\n", containerRuntime)
		return
	}

	fmt.Printf("Remote app: %s\n", renderOrDash(containerName))
	fmt.Printf("container_status: %s\n", containerStatus)
	fmt.Printf("container_image: %s\n", containerImage)
	fmt.Printf("container_runtime: %s\n", containerRuntime)
	fmt.Printf("health_url: %s\n", renderOrDash(cfg.Deploy.HealthcheckURL))
	fmt.Printf("health: %s\n", healthStatus)
	if healthErr != "" {
		fmt.Printf("health_error: %s\n", healthErr)
	}
	if containerStatsErr != nil {
		fmt.Printf("container_runtime_error: %s\n", containerStatsErr.Error())
	}
}

func renderRemoteComposeSummary(runner *sshrunner.Runner, host string, cfg *config.Config) {
	mode, _, composeFileName, err := detectDeployMode(cfgFile)
	if err != nil {
		fmt.Printf("Remote app: skipped (detect deploy mode: %v)\n", err)
		return
	}
	if mode != deployModeCompose {
		fmt.Println("Remote app: compose mode recorded in state, but no compose file in config directory")
		return
	}

	remoteDir := remoteComposeDir(cfg.Project, cfg.SSH.User)
	remoteComposePath := filepath.ToSlash(filepath.Join(remoteDir, composeFileName))
	cmd := fmt.Sprintf("cd %s && docker compose -f %s ps --format '{{.Service}}|{{.State}}|{{.Health}}'", shellQuote(remoteDir), shellQuote(remoteComposePath))
	result, err := runRemoteCommand(runner, host, cmd)
	if err != nil {
		if isTimeoutError(err) {
			fmt.Println("Remote app: compose status unavailable (timed out)")
			return
		}
		fmt.Printf("Remote app: compose status unavailable (%v)\n", err)
		return
	}

	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		fmt.Printf("Remote app: compose stack %q has no services\n", composeFileName)
		return
	}

	if !statsForNerds {
		fmt.Printf("Remote app: compose stack %s\n", composeFileName)
	} else {
		fmt.Printf("Remote app: compose stack %s\n", composeFileName)
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		service := "-"
		stateLabel := "-"
		health := "-"
		if len(parts) > 0 {
			service = renderOrDash(parts[0])
		}
		if len(parts) > 1 {
			stateLabel = renderOrDash(parts[1])
		}
		if len(parts) > 2 {
			health = renderOrDash(parts[2])
		}
		if !statsForNerds {
			fmt.Printf("  - %s: %s (%s)\n", service, stateLabel, health)
		} else {
			fmt.Printf("service %s: state=%s health=%s\n", service, stateLabel, health)
		}
	}

	if statsForNerds {
		statsResult, statsErr := runRemoteCommand(runner, host, fmt.Sprintf("cd %s && docker compose -f %s ps --format '{{.Name}}' | xargs -r docker stats --no-stream --format '{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}'", shellQuote(remoteDir), shellQuote(remoteComposePath)))
		if statsErr != nil {
			if isTimeoutError(statsErr) {
				fmt.Println("compose_runtime_error: timed out")
			} else {
				fmt.Printf("compose_runtime_error: %s\n", statsErr.Error())
			}
		} else {
			for _, line := range strings.Split(strings.TrimSpace(statsResult.Stdout), "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
				if len(parts) != 3 {
					continue
				}
				fmt.Printf("service_runtime %s: cpu=%s mem=%s\n", renderOrDash(parts[0]), renderOrDash(parts[1]), renderOrDash(parts[2]))
			}
		}
	}

	if strings.TrimSpace(cfg.Deploy.HealthcheckURL) == "" {
		return
	}

	if _, err := runRemoteCommand(runner, host, fmt.Sprintf("curl -fsS --max-time 5 %s >/dev/null", shellQuote(cfg.Deploy.HealthcheckURL))); err == nil {
		fmt.Printf("health_url: %s\n", renderOrDash(cfg.Deploy.HealthcheckURL))
		fmt.Println("health: healthy")
		return
	}

	fmt.Printf("health_url: %s\n", renderOrDash(cfg.Deploy.HealthcheckURL))
	if statsForNerds {
		fmt.Println("health: unhealthy")
	} else {
		fmt.Println("  Health: unhealthy")
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(strings.TrimSpace(s), "'", "'\"'\"'") + "'"
}

func resolveRemoteRunnerAndHost(s *state.State, remoteIPv4 string) (*sshrunner.Runner, string, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, "", fmt.Errorf("load config %q: %w", cfgFile, err)
	}

	host := strings.TrimSpace(s.PublicIP)
	if host == "" {
		host = strings.TrimSpace(remoteIPv4)
	}
	if host == "" {
		return nil, "", fmt.Errorf("no public IP available")
	}

	runner, err := sshrunner.NewRunner(sshrunner.Config{
		User:           cfg.SSH.User,
		PrivateKeyPath: cfg.SSH.PrivateKeyPath,
		ConfigDir:      filepath.Dir(cfgFile),
		ConnectTimeout: time.Duration(cfg.SSH.ConnectTimeoutSeconds) * time.Second,
		RetryInterval:  3 * time.Second,
	})
	if err != nil {
		return nil, "", err
	}

	return runner, host, nil
}

func runRemoteScript(runner *sshrunner.Runner, host, script string) (string, error) {
	result, err := runRemoteCommand(runner, host, "sh -lc "+shellQuote(script))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

func runRemoteCommand(runner *sshrunner.Runner, host, command string) (sshrunner.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), statusRemoteCommandTimeout)
	defer cancel()
	return runner.Run(ctx, host, command)
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "signal: killed")
}

func formatMemory(memoryMB int) string {
	if memoryMB <= 0 {
		return "-"
	}
	gib := float64(memoryMB) / 1024.0
	if math.Mod(gib, 1) == 0 {
		return fmt.Sprintf("%.0f GiB", gib)
	}
	return fmt.Sprintf("%.1f GiB", gib)
}

func intToString(v int) string {
	if v <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", v)
}

func currencySymbol(currency string) string {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "EUR":
		return "EUR "
	case "USD":
		return "USD "
	default:
		if strings.TrimSpace(currency) == "" {
			return ""
		}
		return strings.ToUpper(strings.TrimSpace(currency)) + " "
	}
}
