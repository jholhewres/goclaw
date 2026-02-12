package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newConfigCmd cria o comando `copilot config` para gerenciar configurações.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Gerencia configurações do assistente",
		Long: `Gerencia as configurações do AgentGo Copilot.

Exemplos:
  copilot config init
  copilot config show
  copilot config set model gpt-4o`,
	}

	cmd.AddCommand(
		newConfigInitCmd(),
		newConfigShowCmd(),
		newConfigSetCmd(),
	)

	return cmd
}

func newConfigInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Inicializa configuração padrão",
		RunE: func(_ *cobra.Command, _ []string) error {
			// TODO: Gerar config.yaml padrão.
			fmt.Println("Configuração criada em ./config.yaml")
			return nil
		},
	}
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Exibe configuração atual",
		RunE: func(_ *cobra.Command, _ []string) error {
			// TODO: Carregar e exibir config.
			fmt.Println("Nenhuma configuração encontrada. Use 'copilot config init'.")
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Define um valor de configuração",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			// TODO: Atualizar config.
			fmt.Printf("Configuração %q definida para %q\n", args[0], args[1])
			return nil
		},
	}
}
