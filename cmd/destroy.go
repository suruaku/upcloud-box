package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy provisioned resources",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfigOrErr()
		if err != nil {
			return err
		}

		_ = cfg
		fmt.Println("TODO: destroy resources using", cfgFile)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(destroyCmd)
}
