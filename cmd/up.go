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
	deployrunner "github.com/ikox01/upcloud-box/internal/deploy"
	"github.com/ikox01/upcloud-box/internal/infra"
	"github.com/ikox01/upcloud-box/internal/infra/factory"
	sshrunner "github.com/ikox01/upcloud-box/internal/ssh"
	"github.com/ikox01/upcloud-box/internal/state"
	"github.com/spf13/cobra"
)

var upProvisionOnly bool
var upWaitTimeout time.Duration

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Provision if needed and deploy application",
	RunE: func(cmd *cobra.Command, args []string) error {
		logVerbose("starting up flow with config=%s provision-only=%t wait-timeout=%s", cfgFile, upProvisionOnly, upWaitTimeout)
		cfg, bootstrap, err := loadOrBootstrapConfigForUp(cfgFile)
		if err != nil {
			return wrapUserError("load config", err)
		}

		if bootstrap.ConfigCreated {
			fmt.Printf("Up: initialized defaults (%s, %s)\n", cfgFile, bootstrap.CloudInitPath)
			logVerbose("bootstrap details: config_created=%t cloud_init_created=%t cloud_init_path=%s", bootstrap.ConfigCreated, bootstrap.CloudInitCreated, bootstrap.CloudInitPath)
		}

		s, err := loadOrInitState(state.DefaultPath)
		if err != nil {
			return wrapUserError("load state", err)
		}

		if strings.TrimSpace(s.ServerUUID) == "" {
			logVerbose("up flow: no tracked server, provisioning")
			if err := runProvisionFlow(cfg, s, upWaitTimeout); err != nil {
				return wrapUserError("provision flow", err)
			}
		} else {
			logVerbose("up flow: tracked server exists, checking state")
			if err := repairTrackedServerState(s, upWaitTimeout); err != nil {
				return wrapUserError("repair tracked state", err)
			}
			if strings.TrimSpace(s.ServerUUID) == "" {
				logVerbose("up flow: tracked server missing remotely, provisioning fresh server")
				if err := runProvisionFlow(cfg, s, upWaitTimeout); err != nil {
					return wrapUserError("provision flow", err)
				}
			} else {
				logVerbose("up flow: tracked server healthy, skipping provision")
			}
		}

		if upProvisionOnly {
			fmt.Println("Infrastructure ready")
			logVerbose("up flow: provision-only mode, skipping deploy")
			return nil
		}

		if err := runDeployFlow(cfg, s); err != nil {
			return wrapUserError("deploy flow", err)
		}

		if strings.TrimSpace(s.PublicIP) != "" {
			fmt.Printf("App URL: http://%s/\n", strings.TrimSpace(s.PublicIP))
		}
		return nil
	},
}

type upBootstrapResult struct {
	ConfigCreated    bool
	CloudInitCreated bool
	CloudInitPath    string
}

func loadOrBootstrapConfigForUp(path string) (*config.Config, upBootstrapResult, error) {
	cfg, err := config.Load(path)
	if err == nil {
		return cfg, upBootstrapResult{}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, upBootstrapResult{}, err
	}

	bootstrap, err := bootstrapUpDefaults(path)
	if err != nil {
		return nil, upBootstrapResult{}, err
	}

	cfg, err = config.Load(path)
	if err != nil {
		return nil, upBootstrapResult{}, err
	}

	return cfg, bootstrap, nil
}

func bootstrapUpDefaults(path string) (upBootstrapResult, error) {
	defaultCfg := config.Default()
	cloudInitPath := strings.TrimSpace(defaultCfg.Provision.CloudInitPath)
	if cloudInitPath == "" {
		return upBootstrapResult{}, fmt.Errorf("default cloud-init path is empty")
	}

	if err := writeConfig(path, cloudInitPath, false); err != nil {
		return upBootstrapResult{}, err
	}

	result := upBootstrapResult{
		ConfigCreated: true,
		CloudInitPath: cloudInitPath,
	}

	if _, err := os.Stat(cloudInitPath); err == nil {
		return result, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return upBootstrapResult{}, fmt.Errorf("check cloud-init file %q: %w", cloudInitPath, err)
	}

	keys, err := resolveSSHAuthorizedKeys(nil, path)
	if err != nil {
		return upBootstrapResult{}, err
	}

	if err := writeCloudInit(cloudInitPath, defaultCfg.SSH.User, keys, false); err != nil {
		return upBootstrapResult{}, err
	}

	result.CloudInitCreated = true
	return result, nil
}

func init() {
	upCmd.Flags().BoolVar(&upProvisionOnly, "provision-only", false, "run provisioning only, skip deploy")
	upCmd.Flags().DurationVar(&upWaitTimeout, "wait-timeout", 10*time.Minute, "max time to wait for server to become started")
	rootCmd.AddCommand(upCmd)
}

func runProvisionFlow(cfg *config.Config, s *state.State, waitTimeout time.Duration) error {
	fmt.Println("Provisioning infrastructure...")
	logVerbose("up flow: provisioning infrastructure with zone=%s plan=%s template=%s hostname=%s", cfg.UpCloud.Zone, cfg.UpCloud.Plan, cfg.UpCloud.Template, cfg.Provision.Hostname)

	cloudInitRaw, err := readCloudInitPassThrough(cfg.Provision.CloudInitPath)
	if err != nil {
		return wrapUserError("read cloud-init", err)
	}

	provider, err := factory.NewDefaultProvider()
	if err != nil {
		return wrapUserError("initialize provider", err)
	}

	var result infra.ProvisionResult
	if err := runStep("Provisioning server on UpCloud...", upDoneMessage("Server provisioning request accepted"), func() error {
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
	if err := runStep("Waiting for server to become ready...", upDoneMessage("Server is started"), func() error {
		var stepErr error
		serverInfo, stepErr = provider.WaitReady(context.Background(), result.ServerID, waitTimeout)
		return stepErr
	}); err != nil {
		return wrapUserError("wait for server readiness", err)
	}

	s.PublicIP = serverInfo.PublicIPv4
	if err := state.Save(state.DefaultPath, *s); err != nil {
		return wrapUserError("save state", err)
	}

	if err := runStep("Running post-provision SSH and Docker checks...", upDoneMessage("Post-provision checks passed"), func() error {
		return runPostProvisionChecks(cfg, serverInfo.PublicIPv4)
	}); err != nil {
		return wrapUserError("post-provision checks", err)
	}

	logVerbose("up flow: provisioned server_id=%s hostname=%s public_ipv4=%s", serverInfo.ServerID, serverInfo.Hostname, serverInfo.PublicIPv4)
	fmt.Println("Infrastructure ready")
	return nil
}

func repairTrackedServerState(s *state.State, waitTimeout time.Duration) error {
	if strings.TrimSpace(s.ServerUUID) == "" {
		return nil
	}

	if strings.TrimSpace(s.PublicIP) != "" {
		return nil
	}

	provider, err := factory.NewDefaultProvider()
	if err != nil {
		return wrapUserError("initialize provider", err)
	}

	fmt.Printf("Up: server_uuid %s exists but public_ip is missing, repairing state...\n", s.ServerUUID)

	serverInfo, err := provider.Get(context.Background(), s.ServerUUID)
	if err != nil {
		if isLikelyNotFound(err) {
			s.ServerUUID = ""
			s.PublicIP = ""
			if saveErr := state.Save(state.DefaultPath, *s); saveErr != nil {
				return wrapUserError("save state", saveErr)
			}
			return nil
		}
		return wrapUserError("lookup server", err)
	}

	if strings.TrimSpace(serverInfo.PublicIPv4) == "" {
		var waitInfo infra.ServerInfo
		if err := runStep("Waiting for tracked server public IP...", "Tracked server is ready", func() error {
			var stepErr error
			waitInfo, stepErr = provider.WaitReady(context.Background(), s.ServerUUID, waitTimeout)
			return stepErr
		}); err != nil {
			return wrapUserError("wait for tracked server", err)
		}
		serverInfo = waitInfo
	}

	if strings.TrimSpace(serverInfo.PublicIPv4) == "" {
		return fmt.Errorf("tracked server %s has no public IPv4 address", s.ServerUUID)
	}

	s.PublicIP = strings.TrimSpace(serverInfo.PublicIPv4)
	if err := state.Save(state.DefaultPath, *s); err != nil {
		return wrapUserError("save state", err)
	}

	fmt.Printf("Up: repaired state with public_ip %s\n", s.PublicIP)
	return nil
}

func runDeployFlow(cfg *config.Config, s *state.State) error {
	host := strings.TrimSpace(s.PublicIP)
	if host == "" {
		return wrapUserError("validate state", fmt.Errorf("state has no public_ip; run `upcloud-box provision` first"))
	}
	if strings.TrimSpace(s.ServerUUID) == "" {
		return wrapUserError("validate state", fmt.Errorf("state has no server_uuid; run `upcloud-box provision` first"))
	}

	mode, composePath, composeFileName, err := detectDeployMode(cfgFile)
	if err != nil {
		return wrapUserError("detect deploy mode", err)
	}

	if mode == deployModeCompose && hasLikelySingleDeploySettings(cfg) {
		logVerbose("compose file detected; using compose mode (single-container settings ignored)")
	}

	runner, err := sshrunner.NewRunner(sshrunner.Config{
		User:           cfg.SSH.User,
		PrivateKeyPath: cfg.SSH.PrivateKeyPath,
		ConfigDir:      filepath.Dir(cfgFile),
		ConnectTimeout: time.Duration(cfg.SSH.ConnectTimeoutSeconds) * time.Second,
		RetryInterval:  3 * time.Second,
	})
	if err != nil {
		return wrapUserError("create ssh runner", err)
	}

	deployer, err := deployrunner.New(runner)
	if err != nil {
		return wrapUserError("initialize deployer", err)
	}

	if mode == deployModeCompose {
		fmt.Println("Deploying application...")
		if err := runStep("Deploying compose stack and running health checks...", upDoneMessage("Compose deploy completed successfully"), func() error {
			return deployer.RunCompose(context.Background(), deployrunner.ComposeRequest{
				Host:                host,
				ComposeLocalPath:    composePath,
				ComposeFileName:     composeFileName,
				RemoteDir:           remoteComposeDir(cfg.Project, cfg.SSH.User),
				HealthcheckURL:      cfg.Deploy.HealthcheckURL,
				HealthcheckTimeout:  time.Duration(cfg.Deploy.HealthcheckTimeoutSecs) * time.Second,
				HealthcheckInterval: time.Duration(cfg.Deploy.HealthcheckIntervalSecs) * time.Second,
			})
		}); err != nil {
			return wrapUserError("deploy compose stack", err)
		}

		s.LastSuccessfulImage = ""
		s.MarkDeployAt(time.Now())
		s.LastDeployMode = string(deployModeCompose)
		if err := state.Save(state.DefaultPath, *s); err != nil {
			return wrapUserError("save state", err)
		}

		logVerbose("up flow: deployed compose file=%s host=%s", composeFileName, host)
		fmt.Println("Application deployed")
		return nil
	}

	fmt.Println("Deploying application...")
	if err := runStep("Deploying container and running health checks...", upDoneMessage("Deploy completed successfully"), func() error {
		return deployer.Run(context.Background(), deployrunner.Request{
			Host:                host,
			ContainerName:       cfg.Deploy.ContainerName,
			Image:               cfg.Deploy.Image,
			Port:                cfg.Deploy.Port,
			EnvFile:             cfg.Deploy.EnvFile,
			HealthcheckURL:      cfg.Deploy.HealthcheckURL,
			HealthcheckTimeout:  time.Duration(cfg.Deploy.HealthcheckTimeoutSecs) * time.Second,
			HealthcheckInterval: time.Duration(cfg.Deploy.HealthcheckIntervalSecs) * time.Second,
		})
	}); err != nil {
		return wrapUserError("deploy container", err)
	}

	s.MarkDeploy(cfg.Deploy.Image, time.Now())
	s.LastDeployMode = string(deployModeSingle)
	if err := state.Save(state.DefaultPath, *s); err != nil {
		return wrapUserError("save state", err)
	}

	logVerbose("up flow: deployed image=%s host=%s", cfg.Deploy.Image, host)
	fmt.Println("Application deployed")
	return nil
}

func upDoneMessage(message string) string {
	if verbose {
		return message
	}
	return ""
}
