package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ikox01/upcloud-box/internal/infra/factory"
	"github.com/ikox01/upcloud-box/internal/state"
	"github.com/spf13/cobra"
)

var destroyYes bool

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy provisioned resources",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := state.Load(state.DefaultPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fmt.Printf("No state file found at %s; nothing to destroy\n", state.DefaultPath)
				return nil
			}
			return err
		}

		serverID := strings.TrimSpace(s.ServerUUID)
		if serverID == "" {
			fmt.Println("No tracked server in state; nothing to destroy")
			return nil
		}

		if !destroyYes {
			confirmed, err := confirmDestroy(serverID)
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Println("Destroy cancelled")
				return nil
			}
		}

		provider, err := factory.NewDefaultProvider()
		if err != nil {
			return err
		}

		if err := runStep("Destroying server on UpCloud...", "Destroy request completed", func() error {
			return provider.Destroy(context.Background(), serverID)
		}); err != nil {
			if isLikelyNotFound(err) {
				fmt.Printf("Server %s already missing; cleaning local state\n", serverID)
			} else {
				return err
			}
		} else {
			fmt.Printf("Removed server %s\n", serverID)
		}

		s.ServerUUID = ""
		s.PublicIP = ""
		if err := state.Save(state.DefaultPath, *s); err != nil {
			return err
		}

		fmt.Printf("State updated: cleared server_uuid and public_ip in %s\n", state.DefaultPath)
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
