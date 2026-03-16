package cmd

import "github.com/spf13/cobra"

var cfgFile string
var noSpinner bool
var verbose bool
var appVersion = "dev"

var rootCmd = &cobra.Command{
	Use:   "upcloud-app-platform",
	Short: "Deploy and operate apps on UpCloud PaaS",
	Long:  "upcloud-app-platform is a PaaS-oriented CLI that provisions app infrastructure on UpCloud and deploys web or mobile backends without manual server management.",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.Version = appVersion
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "upcloud-app-platform.yaml", "config file path")
	rootCmd.PersistentFlags().BoolVar(&noSpinner, "no-spinner", false, "disable spinner progress output")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose logs and detailed errors")
}
