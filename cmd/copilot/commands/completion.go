package commands

import (
	"os"

	"github.com/spf13/cobra"
)

// newCompletionCmd creates the `copilot completion` command that generates
// shell completion scripts for bash, zsh, fish, and powershell.
func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell auto-completion scripts for copilot.

To load completions:

Bash:
  $ source <(copilot completion bash)
  # To load completions for each session, add to ~/.bashrc:
  echo 'source <(copilot completion bash)' >> ~/.bashrc

Zsh:
  $ source <(copilot completion zsh)
  # To load completions for each session, add to ~/.zshrc:
  echo 'source <(copilot completion zsh)' >> ~/.zshrc

Fish:
  $ copilot completion fish | source
  # To load completions for each session:
  copilot completion fish > ~/.config/fish/completions/copilot.fish

PowerShell:
  PS> copilot completion powershell | Out-String | Invoke-Expression
  # To load completions for each session, add to your profile:
  copilot completion powershell | Out-String | Invoke-Expression`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(os.Stdout, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return nil
		},
	}
	return cmd
}
