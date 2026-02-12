package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newChatCmd cria o comando `copilot chat` para conversas interativas.
func newChatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat [message]",
		Short: "Conversa interativa com o assistente",
		Long: `Inicia uma conversa com o assistente. Pode enviar uma mensagem
direta ou entrar no modo interativo (sem argumentos).

Exemplos:
  copilot chat "O que tenho na agenda hoje?"
  copilot chat  # modo interativo`,
		Args: cobra.MaximumNArgs(1),
		RunE: runChat,
	}

	cmd.Flags().StringP("model", "m", "", "modelo LLM a usar (ex: gpt-4o-mini)")
	return cmd
}

func runChat(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		// Modo single message.
		fmt.Printf("Processando: %s\n", args[0])
		// TODO: Inicializar assistant e processar mensagem.
		fmt.Println("Chat single-shot ainda não implementado.")
		return nil
	}

	// Modo interativo.
	// TODO: Implementar REPL interativo.
	fmt.Println("Modo interativo ainda não implementado.")
	return nil
}
