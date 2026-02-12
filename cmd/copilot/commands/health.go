package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newHealthCmd cria o comando `copilot health` para verificação de saúde.
// Usado pelo Docker HEALTHCHECK e monitoramento.
func newHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Verifica o estado de saúde do serviço",
		Long:  `Retorna o status de saúde do AgentGo Copilot. Usado por Docker HEALTHCHECK e monitoramento.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// TODO: Implementar verificação real (checar canais, scheduler, memória).
			// Por enquanto retorna OK para que o Docker HEALTHCHECK funcione.
			fmt.Println(`{"status":"ok","version":"dev"}`)
			return nil
		},
	}
}
