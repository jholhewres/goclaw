// Package commands implementa os comandos CLI do AgentGo Copilot usando cobra.
package commands

import (
	"github.com/spf13/cobra"
)

// NewRootCmd cria o comando raiz do CLI com todos os subcomandos registrados.
func NewRootCmd(version string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "copilot",
		Short: "AgentGo Copilot - Personal Assistant",
		Long: `AgentGo Copilot é um assistente pessoal open-source em Go.
Funciona como CLI e serviço de mensagens (WhatsApp, Discord, Telegram).

Exemplos:
  copilot chat "O que tenho na agenda hoje?"
  copilot serve --channel whatsapp
  copilot schedule list
  copilot skill search calendar`,
		Version: version,
	}

	// Registra subcomandos.
	rootCmd.AddCommand(
		newChatCmd(),
		newServeCmd(),
		newScheduleCmd(),
		newSkillCmd(),
		newConfigCmd(),
		newRememberCmd(),
	)

	// Flags globais.
	rootCmd.PersistentFlags().StringP("config", "c", "", "caminho para o arquivo de configuração")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "habilita logs detalhados")

	return rootCmd
}
