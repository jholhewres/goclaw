// Package main é o ponto de entrada do CLI do AgentGo Copilot.
// Utiliza cobra para gerenciamento de comandos e viper para configuração.
package main

import (
	"fmt"
	"os"

	"github.com/jholhewres/goclaw/cmd/copilot/commands"
)

// version é injetado em build time via ldflags.
var version = "dev"

func main() {
	rootCmd := commands.NewRootCmd(version)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
		os.Exit(1)
	}
}
