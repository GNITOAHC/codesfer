package main

import (
	"github.com/gnitoahc/codesfer/internal/server"
	"github.com/gnitoahc/codesfer/pkg/version"

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
		cmd.Println("Running codeserver version", version.Version)
		server.Serve(serveFlags)
	},
}

var initCmd = &cobra.Command{
	Use:   "init [dotenv]",
	Short: "Initialize environment for codeserver",
	Long:  "Initialize environment for codeserver. Optionally pass a target dotenv file path to create.",
	Example: "" +
		"  codeserver init\n" +
		"  codeserver init .env.local",
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dotenvPath := ""
		if len(args) > 0 {
			dotenvPath = args[0]
		}
		server.SetupEnv(dotenvPath)
	},
}

func main() {
	rootCmd.AddCommand(serveCmd, initCmd)

	// Serve command flags
	serveCmd.Flags().IntVarP(&serveFlags.Port, "port", "p", 3000, "Port to listen on")
	serveCmd.Flags().StringVar(&serveFlags.Dotenv, "dotenv", ".env", "Path to .env file")

	rootCmd.Execute()
}
