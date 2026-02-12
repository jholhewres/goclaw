package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newSkillCmd cria o comando `copilot skill` para gerenciar skills.
func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Gerencia skills do assistente",
		Long: `Gerencia skills instaladas e disponíveis. Permite buscar,
instalar, listar e atualizar skills.

Exemplos:
  copilot skill list
  copilot skill search calendar
  copilot skill install github.com/goclaw/skills/calendar
  copilot skill update --all`,
	}

	cmd.AddCommand(
		newSkillListCmd(),
		newSkillSearchCmd(),
		newSkillInstallCmd(),
		newSkillUpdateCmd(),
	)

	return cmd
}

func newSkillListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lista skills instaladas",
		RunE: func(_ *cobra.Command, _ []string) error {
			// TODO: Carregar registry e listar skills.
			fmt.Println("Nenhuma skill instalada.")
			return nil
		},
	}
}

func newSkillSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Busca skills disponíveis",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			// TODO: Buscar skills no registry remoto.
			fmt.Printf("Buscando skills por %q...\n", args[0])
			return nil
		},
	}
}

func newSkillInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <name>",
		Short: "Instala uma skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			// TODO: Instalar skill do registry.
			fmt.Printf("Instalando skill %q...\n", args[0])
			return nil
		},
	}
}

func newSkillUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [name]",
		Short: "Atualiza skills instaladas",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			if all {
				// TODO: Atualizar todas as skills do registry.
				fmt.Println("Atualizando todas as skills...")
				return nil
			}
			if len(args) > 0 {
				// TODO: Atualizar skill específica.
				fmt.Printf("Atualizando skill %q...\n", args[0])
				return nil
			}
			return fmt.Errorf("especifique uma skill ou use --all")
		},
	}

	cmd.Flags().Bool("all", false, "atualiza todas as skills")
	return cmd
}
