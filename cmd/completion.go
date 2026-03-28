package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for ralphglasses.

To install completions:

  Bash:
    ralphglasses completion bash > /etc/bash_completion.d/ralphglasses
    # or for current user:
    ralphglasses completion bash > ~/.bash_completion

  Zsh:
    ralphglasses completion zsh > "${fpath[1]}/_ralphglasses"
    # or add to ~/.zshrc:
    source <(ralphglasses completion zsh)

  Fish:
    ralphglasses completion fish > ~/.config/fish/completions/ralphglasses.fish`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		default:
			return fmt.Errorf("unsupported shell: %s (use bash, zsh, or fish)", args[0])
		}
	},
}
