package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var provisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Provision secure Docker host on UpCloud",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfigOrErr()
		if err != nil {
			return err
		}

		_ = cfg
		fmt.Println("TODO: provision server using", cfgFile)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(provisionCmd)
}
