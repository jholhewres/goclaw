package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newScheduleCmd cria o comando `copilot schedule` para gerenciar tarefas agendadas.
func newScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Gerencia tarefas agendadas",
		Long: `Gerencia tarefas agendadas do assistente. Permite adicionar,
remover e listar tarefas que serão executadas automaticamente.

Exemplos:
  copilot schedule list
  copilot schedule add "every weekday 9am" "Send me a daily briefing" --channel whatsapp
  copilot schedule remove <id>`,
	}

	cmd.AddCommand(
		newScheduleListCmd(),
		newScheduleAddCmd(),
		newScheduleRemoveCmd(),
	)

	return cmd
}

func newScheduleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lista todas as tarefas agendadas",
		RunE: func(_ *cobra.Command, _ []string) error {
			// TODO: Carregar scheduler e listar jobs.
			fmt.Println("Nenhuma tarefa agendada.")
			return nil
		},
	}
}

func newScheduleAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <schedule> <command>",
		Short: "Adiciona uma nova tarefa agendada",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			schedule := args[0]
			command := args[1]
			// TODO: Adicionar job ao scheduler.
			fmt.Printf("Tarefa agendada: %q → %q\n", schedule, command)
			return nil
		},
	}

	cmd.Flags().String("channel", "whatsapp", "canal para enviar resultado")
	cmd.Flags().String("chat-id", "", "ID do chat/grupo destino")
	return cmd
}

func newScheduleRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove uma tarefa agendada",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			// TODO: Remover job do scheduler.
			fmt.Printf("Tarefa %q removida.\n", args[0])
			return nil
		},
	}
}
