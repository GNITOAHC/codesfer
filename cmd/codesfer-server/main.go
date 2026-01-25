package main

import (
	"codesfer/internal/server"
	"codesfer/pkg/version"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "codeserver",
	Short:   "Codeserver is a server for self-hosted code sharing",
	Version: version.Version,
}

var serveFlags server.ServeFlags
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the codeserver",
	Run: func(cmd *cobra.Command, args []string) {
		server.Serve(serveFlags)
	},
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize environment for codeserver",
	Run: func(cmd *cobra.Command, args []string) {
		server.SetupEnv()
	},
}

func main() {
	rootCmd.AddCommand(serveCmd, initCmd)

	// Serve command flags
	serveCmd.Flags().IntVarP(&serveFlags.Port, "port", "p", 3000, "Port to listen on")

	rootCmd.Execute()
}
