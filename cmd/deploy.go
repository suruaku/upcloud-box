package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	deployrunner "github.com/ikox01/upcloud-box/internal/deploy"
	sshrunner "github.com/ikox01/upcloud-box/internal/ssh"
	"github.com/ikox01/upcloud-box/internal/state"
	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy single container workload",
	RunE: func(cmd *cobra.Command, args []string) error {
		logVerbose("starting deploy with config=%s", cfgFile)
		cfg, err := loadConfigOrErr()
		if err != nil {
			return err
		}

		s, err := state.Load(state.DefaultPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return wrapUserError("load state", fmt.Errorf("state file %s not found; run provision first", state.DefaultPath))
			}
			return wrapUserError("load state", err)
		}

		host := strings.TrimSpace(s.PublicIP)
		if host == "" {
			return wrapUserError("validate state", fmt.Errorf("state has no public_ip; run provision first"))
		}
		if strings.TrimSpace(s.ServerUUID) == "" {
			return wrapUserError("validate state", fmt.Errorf("state has no server_uuid; run provision first"))
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

		if err := runStep("Deploying container and running health checks...", "Deploy completed successfully", func() error {
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
		if err := state.Save(state.DefaultPath, *s); err != nil {
			return wrapUserError("save state", err)
		}

		fmt.Printf("Deployed image %s to %s\n", cfg.Deploy.Image, host)
		fmt.Printf("State saved to %s\n", state.DefaultPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)
}
