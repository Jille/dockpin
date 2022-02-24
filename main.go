package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	rootCmd = &cobra.Command{
		Use:     "dockpin {docker|apt} {pin|install}",
		Short:   "A tool for pinning Docker image and apt package versions",
		Version: "dev", // AUTOMATION-ANCHOR: This line will be automatically replaced during releases.
	}
)

func main() {
	aptCmd.AddCommand(aptPinCmd)
	aptCmd.AddCommand(aptInstallCmd)
	dockerCmd.AddCommand(dockerPinCmd)
	dockerCmd.AddCommand(dockerBaseCmd)
	rootCmd.AddCommand(aptCmd)
	rootCmd.AddCommand(dockerCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
