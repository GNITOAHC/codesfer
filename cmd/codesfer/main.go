package main

import (
	"fmt"
	"github.com/gnitoahc/codesfer/internal/cli"
	"github.com/gnitoahc/codesfer/pkg/version"
	"strconv"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "codesfer",
	Short:   "Codesfer is a tool for sending and receiving code snippets",
	Long:    `Codesfer is a tool for sending and receiving code snippets. It allows you to share code snippets with others easily and quickly.`,
	Version: version.Version,
}

var pushCmdFlags cli.PushFlags
var pushCmd = &cobra.Command{
	Use:   "push [file1] [file2] ...",
	Short: "Send a code snippet",
	Long:  `Send a code snippet. This command allows you to send a code snippet to another user.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cli.Push(pushCmdFlags, args)
	},
}

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all your code snippets",
	Long:    `List all your code snippets. This command allows you to list your code snippets.`,
	Aliases: []string{"ls"},
	Run: func(cmd *cobra.Command, args []string) {
		cli.List()
	},
}

var pullCmdFlags cli.PullFlags
var pullCmd = &cobra.Command{
	Use:   "pull [code]",
	Short: "Receive a code snippet",
	Long:  `Receive a code snippet. This command allows you to receive a code snippet from another user.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.Pull(pullCmdFlags, args[0])
	},
}

var removeCmd = &cobra.Command{
	Use:     "remove [code1] [code2] ...",
	Short:   "Remove a code snippet",
	Aliases: []string{"rm"},
	Run: func(cmd *cobra.Command, args []string) {
		cli.Remove(args)
	},
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication (login, logout, register)",
	Long:  `Manage authentication. Use subcommands to login, logout, or register.`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to Codesfer",
	Long:  `Login to Codesfer. This command allows you to login to Codesfer.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.Login()
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout [session_number]",
	Short: "Logout from Codesfer",
	Long: `Logout from Codesfer. This command allows you to logout from Codesfer.
Run 'codesfer logout' to logout the current machine.
Run 'codesfer logout <number>' to logout a specific session (use 'codesfer account' to see session numbers).`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			cli.Logout(-1)
			return
		}
		sessionIndex, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid session number. Please provide a valid number.")
			return
		}
		cli.Logout(sessionIndex)
	},
}

var authRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register to Codesfer",
	Long:  `Register to Codesfer. This command allows you to register to Codesfer.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.Register()
	},
}

var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Manage your account",
	Long:  `Manage your account. This command allows you to manage your account.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.Account()
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure Codesfer settings",
	Long:  `Configure Codesfer settings. This command allows you to configure Codesfer settings.`,
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a configuration value",
	Long:  `Set a configuration value. This command allows you to set a configuration value.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.ConfigSet()
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a configuration value",
	Long:  `Get a configuration value. This command allows you to get a configuration value.`,
	Run: func(cmd *cobra.Command, args []string) {
		cli.ConfigGet()
	},
}

var inspectCmdFlags cli.InspectFlags
var inspectCmd = &cobra.Command{
	Use:   "inspect [key]",
	Short: "Inspect a code snippet's metadata",
	Long:  `Inspect a code snippet's metadata without downloading. Shows file tree and description.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cli.Inspect(inspectCmdFlags, args[0])
	},
}

func main() {
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
	pushCmd.Flags().BoolVarP(
		&pushCmdFlags.Force, "force", "f", false, "Overwrite existing key if exists, only use it when you want to replace an existing key",
	)
	pushCmd.Flags().StringVarP(
		&pushCmdFlags.Access, "access", "a", "",
		"Access scope: owner, authenticated or public (default: public)",
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
	pullCmd.Flags().StringVarP(
		&pullCmdFlags.File, "file", "f", "", "Extract only this path from the archive (e.g. dir/subdir/file.txt)",
	)

	// ================
	// inspectCmd flags
	// ================
	inspectCmd.Flags().StringVarP(
		&inspectCmdFlags.Pass, "pass", "p", "", "Password for protected snippets",
	)
	inspectCmd.Flags().BoolVar(
		&inspectCmdFlags.JSON, "json", false, "Output raw metadata as JSON",
	)
	inspectCmd.Flags().IntVarP(
		&inspectCmdFlags.Level, "level", "l", 2, "Tree display depth (0 for unlimited)",
	)

	// ===========
	// subcommands
	// ===========
	authCmd.AddCommand(authLoginCmd, authLogoutCmd, authRegisterCmd)
	configCmd.AddCommand(configSetCmd, configGetCmd)
	rootCmd.AddCommand(pushCmd, listCmd, pullCmd, removeCmd, accountCmd, authCmd, configCmd, inspectCmd)

	rootCmd.Execute()
}
