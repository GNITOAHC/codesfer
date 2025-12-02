package main

import (
	"codesfer/internal/cli"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "codesfer",
	Short: "Codesfer is a tool for sending and receiving code snippets.",
	Long:  `Codesfer is a tool for sending and receiving code snippets. It allows you to share code snippets with others easily and quickly.`,
}

var pushCmdFlags cli.PushFlags
var pushCmd = &cobra.Command{
	Use:   "push [file1] [file2] ...",
	Short: "Send a code snippet.",
	Long:  `Send a code snippet. This command allows you to send a code snippet to another user.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.Push(pushCmdFlags, args)
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all your code snippets.",
	Long:  `List all your code snippets. This command allows you to list your code snippets.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.List()
	},
}

var pullCmdFlags cli.PullFlags
var pullCmd = &cobra.Command{
	Use:   "pull [code]",
	Short: "Receive a code snippet.",
	Long:  `Receive a code snippet. This command allows you to receive a code snippet from another user.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.Pull(pullCmdFlags, args[0])
	},
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to Codesfer.",
	Long:  `Login to Codesfer. This command allows you to login to Codesfer.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.Login()
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from Codesfer.",
	Long:  `Logout from Codesfer. This command allows you to logout from Codesfer.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.Logout()
	},
}

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Register to Codesfer.",
	Long:  `Register to Codesfer. This command allows you to register to Codesfer.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.Register()
	},
}

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Manage your account.",
	Long:  `Manage your account. This command allows you to manage your account.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.Account()
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure Codesfer settings.",
	Long:  `Configure Codesfer settings. This command allows you to configure Codesfer settings.`,
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a configuration value.",
	Long:  `Set a configuration value. This command allows you to set a configuration value.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.ConfigSet()
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a configuration value.",
	Long:  `Get a configuration value. This command allows you to get a configuration value.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.ConfigGet()
	},
}

func main() {
	rootCmd.AddCommand(pushCmd, listCmd, pullCmd, loginCmd, logoutCmd, registerCmd, accountCmd)

	// =============
	// pushCmd flags
	// =============
	pushCmd.Flags().StringVarP(
		&pushCmdFlags.Path, "path", "p", "",
		"Path, use slashes to separate folders. e.g. 'folder1/folder2/name', extension is omitted",
	)
	pushCmd.Flags().StringVarP(
		&pushCmdFlags.Desc, "desc", "d", "", "Description of the code snippet",
	)
	pushCmd.Flags().StringVar(
		&pushCmdFlags.Pass, "pass", "", "Password to encrypt the code snippet",
	)
	pushCmd.Flags().StringVarP(
		&pushCmdFlags.Key, "key", "k", "", "Key to get faster access to the code snippet",
	)

	// =============
	// pullCmd flags
	// =============
	pullCmd.Flags().StringVarP(
		&pullCmdFlags.Out, "out", "o", ".", "Output directory",
	)
	pullCmd.Flags().StringVarP(
		&pullCmdFlags.Pass, "pass", "p", "", "Password for the code snippet if it is encrypted",
	)

	// =====================
	// configCmd subcommands
	// =====================
	configCmd.AddCommand(configSetCmd, configGetCmd)
	rootCmd.AddCommand(configCmd)

	rootCmd.Execute()
}
