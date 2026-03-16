package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/suruaku/upcloud-app-platform/internal/infra/factory"
	"github.com/suruaku/upcloud-app-platform/internal/state"
)

var destroyYes bool

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy provisioned resources",
	RunE: func(cmd *cobra.Command, args []string) error {
		logVerbose("starting destroy with config=%s", cfgFile)
		s, err := state.Load(state.DefaultPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fmt.Println("Nothing to destroy")
				logVerbose("destroy skipped: state file %s does not exist", state.DefaultPath)
				return nil
			}
			return wrapUserError("load state", err)
		}

		serverID := strings.TrimSpace(s.ServerUUID)
		if serverID == "" {
			fmt.Println("Nothing to destroy")
			logVerbose("destroy skipped: no tracked server in %s", state.DefaultPath)
			return nil
		}

		if !destroyYes {
			confirmed, err := confirmDestroy(serverID)
			if err != nil {
				return wrapUserError("confirm destroy", err)
			}
			if !confirmed {
				fmt.Println("Cancelled")
				return nil
			}
		}

		provider, err := factory.NewDefaultProvider()
		if err != nil {
			return wrapUserError("initialize provider", err)
		}

		fmt.Println("Destroying infrastructure...")
		if err := runStep("Destroying server on UpCloud...", destroyDoneMessage("Destroy request completed"), func() error {
			return provider.Destroy(context.Background(), serverID)
		}); err != nil {
			if isLikelyNotFound(err) {
				logVerbose("destroy skipped remotely: server %s already missing", serverID)
			} else {
				return wrapUserError("destroy server", err)
			}
		}

		s.ServerUUID = ""
		s.PublicIP = ""
		if err := state.Save(state.DefaultPath, *s); err != nil {
			return wrapUserError("save state", err)
		}

		logVerbose("state updated: cleared server_uuid and public_ip in %s", state.DefaultPath)
		fmt.Println("Infrastructure removed")
		return nil
	},
}

func init() {
	destroyCmd.Flags().BoolVar(&destroyYes, "yes", false, "skip confirmation prompt")
	rootCmd.AddCommand(destroyCmd)
}

func confirmDestroy(serverID string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Destroy server %s? [y/N]: ", serverID)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func destroyDoneMessage(message string) string {
	if verbose {
		return message
	}
	return ""
}
