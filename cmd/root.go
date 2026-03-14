package cmd

import "github.com/spf13/cobra"

var cfgFile string
var noSpinner bool
var verbose bool

var rootCmd = &cobra.Command{
	Use:   "upcloud-box",
	Short: "Provision and deploy a secure Docker host on UpCloud",
	Long:  "upcloud-box provisions a hardened Docker host on UpCloud and deploys a single container workload.",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "upcloud-box.yaml", "config file path")
	rootCmd.PersistentFlags().BoolVar(&noSpinner, "no-spinner", false, "disable spinner progress output")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose logs and detailed errors")
}
