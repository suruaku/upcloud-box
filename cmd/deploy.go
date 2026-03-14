package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy single container workload",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfigOrErr()
		if err != nil {
			return err
		}

		_ = cfg
		fmt.Println("TODO: deploy container using", cfgFile)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)
}
