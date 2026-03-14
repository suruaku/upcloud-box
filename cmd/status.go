package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show infrastructure and app status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfigOrErr()
		if err != nil {
			return err
		}

		_ = cfg
		fmt.Println("TODO: show status using", cfgFile)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
