package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/suruaku/upcloud-app-platform/internal/state"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy container workload or compose stack",
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

		if err := runDeployFlow(cfg, s); err != nil {
			return wrapUserError("deploy flow", err)
		}

		fmt.Printf("State saved to %s\n", state.DefaultPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)
}
