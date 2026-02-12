package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newRememberCmd cria o comando `copilot remember` para adicionar fatos à memória.
func newRememberCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remember <fact>",
		Short: "Adiciona um fato à memória de longo prazo",
		Long: `Adiciona um fato que o assistente deve lembrar em conversas futuras.
Útil para preferências, contexto pessoal e informações recorrentes.

Exemplos:
  copilot remember "I prefer responses in Portuguese"
  copilot remember "My daily standup is at 9am"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			fact := strings.Join(args, " ")
			// TODO: Salvar fato na memória persistente.
			fmt.Printf("Fato memorizado: %q\n", fact)
			return nil
		},
	}
}
