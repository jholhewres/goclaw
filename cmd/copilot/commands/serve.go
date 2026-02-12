package commands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jholhewres/goclaw/pkg/goclaw/copilot"
	"github.com/spf13/cobra"
)

// newServeCmd cria o comando `copilot serve` que inicia o daemon com canais.
func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Inicia o daemon com canais de mensagem",
		Long: `Inicia o AgentGo Copilot como serviço daemon, conectando aos
canais habilitados (WhatsApp, Discord, Telegram) e processando mensagens.

Exemplos:
  copilot serve
  copilot serve --channel whatsapp
  copilot serve --config ./config.yaml`,
		RunE: runServe,
	}

	cmd.Flags().StringSlice("channel", nil, "canais a habilitar (whatsapp, discord, telegram)")
	return cmd
}

func runServe(cmd *cobra.Command, _ []string) error {
	// Configura logger estruturado.
	verbose, err := cmd.Root().PersistentFlags().GetBool("verbose")
	if err != nil {
		verbose = false
	}
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	// Carrega configuração.
	// TODO: Implementar carregamento real via viper quando --config for especificado.
	configPath, _ := cmd.Root().PersistentFlags().GetString("config")
	if configPath != "" {
		logger.Info("carregando configuração", "path", configPath)
		// TODO: Carregar config do arquivo YAML.
	}
	cfg := copilot.DefaultConfig()

	// Cria o assistente.
	assistant := copilot.New(cfg, logger)

	// Inicia com contexto cancelável.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TODO: Registrar canais habilitados no channel manager.
	// Canais serão adicionados conforme implementação individual.

	if err := assistant.Start(ctx); err != nil {
		return fmt.Errorf("falha ao iniciar copilot: %w", err)
	}

	// Aguarda sinal de shutdown (SIGINT, SIGTERM).
	logger.Info("AgentGo Copilot rodando. Pressione Ctrl+C para encerrar.")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("sinal de shutdown recebido, encerrando...")
	assistant.Stop()

	return nil
}
