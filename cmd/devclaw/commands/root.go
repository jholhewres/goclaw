// Package commands implementa os comandos CLI do DevClaw usando cobra.
package commands

import (
	"github.com/spf13/cobra"
)

// NewRootCmd cria o comando raiz do CLI com todos os subcomandos registrados.
func NewRootCmd(version string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "devclaw",
		Short: "DevClaw - AI Agent for Tech Teams",
		Long: `DevClaw is an open-source AI agent for tech teams.
Works as CLI, daemon service, and messaging channels (WhatsApp, Discord, Telegram).

Examples:
  devclaw chat "What time is it?"
  devclaw serve --channel whatsapp
  devclaw schedule list
  devclaw skill search calendar`,
		Version: version,
	}

	// Registra subcomandos.
	rootCmd.AddCommand(
		newChatCmd(),
		newServeCmd(),
		newSetupCmd(),
		newScheduleCmd(),
		newSkillCmd(),
		newConfigCmd(),
		newRememberCmd(),
		newHealthCmd(),
		newChangelogCmd(version),
		newCompletionCmd(),
		newFixCmd(),
		newExplainCmd(),
		newDiffCmd(),
		newCommitCmd(),
		newHowCmd(),
		newShellHookCmd(),
		newMCPCmd(),
	)

	// Flags globais.
	rootCmd.PersistentFlags().StringP("config", "c", "", "caminho para o arquivo de configuração")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "habilita logs detalhados")

	return rootCmd
}
