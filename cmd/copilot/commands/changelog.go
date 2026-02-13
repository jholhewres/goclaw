// Package commands – changelog.go implements the `copilot changelog` CLI command
// that displays the changelog for the current version.
package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// changelogFile is the path to CHANGELOG.md relative to the working directory.
const changelogFile = "CHANGELOG.md"

func newChangelogCmd(version string) *cobra.Command {
	var showAll bool

	cmd := &cobra.Command{
		Use:   "changelog",
		Short: "Show changelog for the current version",
		Long: `Display the changelog entry for the current running version.
Use --all to show the full changelog.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := os.ReadFile(changelogFile)
			if err != nil {
				return fmt.Errorf("could not read %s: %w", changelogFile, err)
			}
			content := string(raw)

			// Show current version header.
			cleanVersion := cleanVersionTag(version)
			fmt.Printf("GoClaw %s\n\n", version)

			if showAll {
				fmt.Println(content)
				return nil
			}

			// Extract the section for the current version.
			section := extractVersionSection(content, cleanVersion)
			if section == "" {
				// Try without the "v" prefix.
				section = extractVersionSection(content, strings.TrimPrefix(cleanVersion, "v"))
			}
			if section == "" {
				fmt.Println("No changelog entry found for this version.")
				fmt.Println("Use --all to see the full changelog.")
				return nil
			}

			fmt.Println(section)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "show full changelog")

	return cmd
}

// cleanVersionTag extracts the semver part from a git describe tag.
// e.g. "v1.1.0-3-g174ea25" → "1.1.0", "v1.1.0" → "1.1.0", "dev" → "dev"
func cleanVersionTag(version string) string {
	v := strings.TrimPrefix(version, "v")

	// Handle git describe format: "1.1.0-3-gabcdef"
	parts := strings.SplitN(v, "-", 2)
	return parts[0]
}

// extractVersionSection extracts the changelog block for a specific version.
// It finds the "## [version]" header and captures everything until the next
// "## [" header or end of file.
func extractVersionSection(content, version string) string {
	lines := strings.Split(content, "\n")

	// Find the start line matching this version.
	startIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Match "## [1.1.0]" with optional date suffix.
		if strings.HasPrefix(trimmed, "## ["+version+"]") {
			startIdx = i
			break
		}
	}

	if startIdx < 0 {
		return ""
	}

	// Find the end (next "## [" header or EOF).
	endIdx := len(lines)
	for i := startIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## [") {
			endIdx = i
			break
		}
	}

	// Join and trim trailing whitespace.
	section := strings.Join(lines[startIdx:endIdx], "\n")
	return strings.TrimRight(section, "\n\t ")
}
