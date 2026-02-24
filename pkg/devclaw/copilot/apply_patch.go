// Package copilot – apply_patch.go implements a robust multi-file patch applicator.
// This is based on the OpenClaw apply_patch tool, allowing the agent to make
// surgical edits to files without having to rewrite the entire file or use
// fragile bash/sed commands.
package copilot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	BeginPatchMarker         = "*** Begin Patch"
	EndPatchMarker           = "*** End Patch"
	AddFileMarker            = "*** Add File: "
	DeleteFileMarker         = "*** Delete File: "
	UpdateFileMarker         = "*** Update File: "
	MoveToMarker             = "*** Move to: "
	EOFMarker                = "*** End of File"
	ChangeContextMarker      = "@@ "
	EmptyChangeContextMarker = "@@"
)

// HunkKind denotes the type of patch operation.
type HunkKind string

const (
	HunkAdd    HunkKind = "add"
	HunkDelete HunkKind = "delete"
	HunkUpdate HunkKind = "update"
)

// AddFileHunk represents adding a new file.
type AddFileHunk struct {
	Path     string
	Contents string
}

// DeleteFileHunk represents deleting an existing file.
type DeleteFileHunk struct {
	Path string
}

// UpdateFileChunk represents a single chunk of changes within an UpdateFileHunk.
type UpdateFileChunk struct {
	ChangeContext string
	OldLines      []string
	NewLines      []string
	IsEndOfFile   bool
}

// UpdateFileHunk represents an update to an existing file, optionally moving it.
type UpdateFileHunk struct {
	Path     string
	MovePath string
	Chunks   []UpdateFileChunk
}

// Hunk encapsulates one of Add, Delete, or Update operations.
type Hunk struct {
	Kind   HunkKind
	Add    *AddFileHunk
	Delete *DeleteFileHunk
	Update *UpdateFileHunk
}

// ApplyPatchResult holds the summary of the applied patch.
type ApplyPatchResult struct {
	Added    []string
	Modified []string
	Deleted  []string
	Text     string
}

// RegisterApplyPatchTool registers the apply_patch tool in the executor.
func RegisterApplyPatchTool(executor *ToolExecutor) {
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "apply_patch",
			Description: "Apply a patch to one or more files using the apply_patch format. The input MUST start with '*** Begin Patch' and end with '*** End Patch'. Used for surgical multi-file edits.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{
						"type":        "string",
						"description": "Patch content using the *** Begin Patch/End Patch format.",
					},
					"working_dir": map[string]any{
						"type":        "string",
						"description": "Optional working directory to resolve relative paths against. Defaults to the current directory.",
					},
				},
				"required": []string{"input"},
			}),
		},
	}, func(ctx context.Context, args map[string]any) (any, error) {
		input, _ := args["input"].(string)
		if strings.TrimSpace(input) == "" {
			return nil, errors.New("provide a patch input")
		}

		cwd := "."
		if wd, ok := args["working_dir"].(string); ok && wd != "" {
			cwd = wd
		}

		result, err := applyPatch(ctx, input, cwd)
		if err != nil {
			return nil, fmt.Errorf("patch failed: %w", err)
		}

		return result.Text, nil
	})
}

func applyPatch(ctx context.Context, input string, cwd string) (*ApplyPatchResult, error) {
	hunks, err := parsePatchText(input)
	if err != nil {
		return nil, err
	}
	if len(hunks) == 0 {
		return nil, errors.New("no files were modified in patch")
	}

	summary := &ApplyPatchResult{
		Added:    []string{},
		Modified: []string{},
		Deleted:  []string{},
	}
	seenAdded := make(map[string]bool)
	seenModified := make(map[string]bool)
	seenDeleted := make(map[string]bool)

	for _, hunk := range hunks {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		switch hunk.Kind {
		case HunkAdd:
			targetPath, err := resolvePatchPath(hunk.Add.Path, cwd)
			if err != nil {
				return nil, err
			}
			if err := ensureDir(targetPath); err != nil {
				return nil, fmt.Errorf("ensuring dir for %s: %w", targetPath, err)
			}
			if err := os.WriteFile(targetPath, []byte(hunk.Add.Contents), 0644); err != nil {
				return nil, fmt.Errorf("writing file %s: %w", targetPath, err)
			}
			recordSummary(summary, seenAdded, "added", hunk.Add.Path)

		case HunkDelete:
			targetPath, err := resolvePatchPath(hunk.Delete.Path, cwd)
			if err != nil {
				return nil, err
			}
			if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("deleting file %s: %w", targetPath, err)
			}
			recordSummary(summary, seenDeleted, "deleted", hunk.Delete.Path)

		case HunkUpdate:
			targetPath, err := resolvePatchPath(hunk.Update.Path, cwd)
			if err != nil {
				return nil, err
			}
			applied, err := applyUpdateHunk(targetPath, hunk.Update.Chunks)
			if err != nil {
				return nil, fmt.Errorf("updating file %s: %w", targetPath, err)
			}

			if hunk.Update.MovePath != "" {
				moveTarget, err := resolvePatchPath(hunk.Update.MovePath, cwd)
				if err != nil {
					return nil, err
				}
				if err := ensureDir(moveTarget); err != nil {
					return nil, fmt.Errorf("ensuring dir for %s: %w", moveTarget, err)
				}
				if err := os.WriteFile(moveTarget, []byte(applied), 0644); err != nil {
					return nil, fmt.Errorf("writing moved file %s: %w", moveTarget, err)
				}
				// Remove old file if it's a different path
				if targetPath != moveTarget {
					if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
						return nil, fmt.Errorf("deleting old file %s during move: %w", targetPath, err)
					}
				}
				recordSummary(summary, seenModified, "modified", hunk.Update.MovePath)
			} else {
				if err := os.WriteFile(targetPath, []byte(applied), 0644); err != nil {
					return nil, fmt.Errorf("writing updated file %s: %w", targetPath, err)
				}
				recordSummary(summary, seenModified, "modified", hunk.Update.Path)
			}
		}
	}

	summary.Text = formatSummary(summary)
	return summary, nil
}

func recordSummary(summary *ApplyPatchResult, seen map[string]bool, bucket string, path string) {
	if seen[path] {
		return
	}
	seen[path] = true
	switch bucket {
	case "added":
		summary.Added = append(summary.Added, path)
	case "modified":
		summary.Modified = append(summary.Modified, path)
	case "deleted":
		summary.Deleted = append(summary.Deleted, path)
	}
}

func formatSummary(summary *ApplyPatchResult) string {
	var sb strings.Builder
	sb.WriteString("Success. Updated the following files:\n")
	for _, f := range summary.Added {
		sb.WriteString(fmt.Sprintf("A %s\n", f))
	}
	for _, f := range summary.Modified {
		sb.WriteString(fmt.Sprintf("M %s\n", f))
	}
	for _, f := range summary.Deleted {
		sb.WriteString(fmt.Sprintf("D %s\n", f))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func resolvePatchPath(filePath string, cwd string) (string, error) {
	// Reject absolute paths - force sandbox to cwd
	if filepath.IsAbs(filePath) {
		return "", fmt.Errorf("absolute paths not allowed in patch: %s", filePath)
	}

	resolved := filepath.Clean(filepath.Join(cwd, filePath))

	// Verify path doesn't escape cwd (prevent path traversal)
	rel, err := filepath.Rel(cwd, resolved)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path %s: %w", filePath, err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes workspace: %s", filePath)
	}

	return resolved, nil
}

func ensureDir(filePath string) error {
	dir := filepath.Dir(filePath)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}

// ----------------------------------------------------------------------------
// Parsing Logic
// ----------------------------------------------------------------------------

func parsePatchText(input string) ([]Hunk, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("invalid patch: input is empty")
	}

	// Normalize CRLF to LF
	input = strings.ReplaceAll(input, "\r\n", "\n")
	lines := strings.Split(input, "\n")

	validated, err := checkPatchBoundariesLenient(lines)
	if err != nil {
		return nil, err
	}

	var hunks []Hunk
	lastLineIndex := len(validated) - 1
	remaining := validated[1:lastLineIndex]
	lineNumber := 2

	for len(remaining) > 0 {
		hunk, consumed, err := parseOneHunk(remaining, lineNumber)
		if err != nil {
			return nil, err
		}
		hunks = append(hunks, hunk)
		lineNumber += consumed
		remaining = remaining[consumed:]
	}

	return hunks, nil
}

func checkPatchBoundariesLenient(lines []string) ([]string, error) {
	strictError := checkPatchBoundariesStrict(lines)
	if strictError == nil {
		return lines, nil
	}

	if len(lines) < 4 {
		return nil, strictError
	}

	first := strings.TrimSpace(lines[0])
	last := strings.TrimSpace(lines[len(lines)-1])

	if (first == "<<EOF" || first == "<<'EOF'" || first == "<<\"EOF\"") && strings.HasSuffix(last, "EOF") {
		inner := lines[1 : len(lines)-1]
		innerError := checkPatchBoundariesStrict(inner)
		if innerError == nil {
			return inner, nil
		}
		return nil, innerError
	}

	return nil, strictError
}

func checkPatchBoundariesStrict(lines []string) error {
	if len(lines) < 2 {
		return errors.New("patch must contain at least a begin and end marker")
	}

	firstLine := strings.TrimSpace(lines[0])
	lastLine := strings.TrimSpace(lines[len(lines)-1])

	if firstLine == BeginPatchMarker && lastLine == EndPatchMarker {
		return nil
	}
	if firstLine != BeginPatchMarker {
		return errors.New("the first line of the patch must be '*** Begin Patch'")
	}
	return errors.New("the last line of the patch must be '*** End Patch'")
}

func parseOneHunk(lines []string, lineNumber int) (Hunk, int, error) {
	if len(lines) == 0 {
		return Hunk{}, 0, fmt.Errorf("invalid patch hunk at line %d: empty hunk", lineNumber)
	}

	firstLine := strings.TrimSpace(lines[0])

	if strings.HasPrefix(firstLine, AddFileMarker) {
		targetPath := firstLine[len(AddFileMarker):]
		var contents strings.Builder
		consumed := 1
		for _, addLine := range lines[1:] {
			if strings.HasPrefix(addLine, "+") {
				contents.WriteString(addLine[1:])
				contents.WriteString("\n")
				consumed++
			} else {
				break
			}
		}
		return Hunk{
			Kind: HunkAdd,
			Add: &AddFileHunk{
				Path:     targetPath,
				Contents: contents.String(),
			},
		}, consumed, nil
	}

	if strings.HasPrefix(firstLine, DeleteFileMarker) {
		targetPath := firstLine[len(DeleteFileMarker):]
		return Hunk{
			Kind:   HunkDelete,
			Delete: &DeleteFileHunk{Path: targetPath},
		}, 1, nil
	}

	if strings.HasPrefix(firstLine, UpdateFileMarker) {
		targetPath := firstLine[len(UpdateFileMarker):]
		remaining := lines[1:]
		consumed := 1
		var movePath string

		if len(remaining) > 0 {
			moveCandidate := strings.TrimSpace(remaining[0])
			if strings.HasPrefix(moveCandidate, MoveToMarker) {
				movePath = moveCandidate[len(MoveToMarker):]
				remaining = remaining[1:]
				consumed++
			}
		}

		var chunks []UpdateFileChunk
		for len(remaining) > 0 {
			if strings.TrimSpace(remaining[0]) == "" {
				remaining = remaining[1:]
				consumed++
				continue
			}
			if strings.HasPrefix(remaining[0], "***") {
				break
			}
			chunk, chunkLines, err := parseUpdateFileChunk(remaining, lineNumber+consumed, len(chunks) == 0)
			if err != nil {
				return Hunk{}, 0, err
			}
			chunks = append(chunks, chunk)
			remaining = remaining[chunkLines:]
			consumed += chunkLines
		}

		if len(chunks) == 0 {
			return Hunk{}, 0, fmt.Errorf("invalid patch hunk at line %d: Update file hunk for path '%s' is empty", lineNumber, targetPath)
		}

		return Hunk{
			Kind: HunkUpdate,
			Update: &UpdateFileHunk{
				Path:     targetPath,
				MovePath: movePath,
				Chunks:   chunks,
			},
		}, consumed, nil
	}

	return Hunk{}, 0, fmt.Errorf("invalid patch hunk at line %d: '%s' is not a valid hunk header. Valid hunk headers: '*** Add File: {path}', '*** Delete File: {path}', '*** Update File: {path}'", lineNumber, lines[0])
}

func parseUpdateFileChunk(lines []string, lineNumber int, allowMissingContext bool) (UpdateFileChunk, int, error) {
	if len(lines) == 0 {
		return UpdateFileChunk{}, 0, fmt.Errorf("invalid patch hunk at line %d: Update hunk does not contain any lines", lineNumber)
	}

	var changeContext string
	startIndex := 0

	if lines[0] == EmptyChangeContextMarker {
		startIndex = 1
	} else if strings.HasPrefix(lines[0], ChangeContextMarker) {
		changeContext = lines[0][len(ChangeContextMarker):]
		startIndex = 1
	} else if !allowMissingContext {
		return UpdateFileChunk{}, 0, fmt.Errorf("invalid patch hunk at line %d: Expected update hunk to start with a @@ context marker, got: '%s'", lineNumber, lines[0])
	}

	if startIndex >= len(lines) {
		return UpdateFileChunk{}, 0, fmt.Errorf("invalid patch hunk at line %d: Update hunk does not contain any lines", lineNumber+1)
	}

	chunk := UpdateFileChunk{
		ChangeContext: changeContext,
		OldLines:      []string{},
		NewLines:      []string{},
		IsEndOfFile:   false,
	}

	parsedLines := 0
	for _, line := range lines[startIndex:] {
		if line == EOFMarker {
			if parsedLines == 0 {
				return UpdateFileChunk{}, 0, fmt.Errorf("invalid patch hunk at line %d: Update hunk does not contain any lines", lineNumber+1)
			}
			chunk.IsEndOfFile = true
			parsedLines++
			break
		}

		if len(line) == 0 {
			chunk.OldLines = append(chunk.OldLines, "")
			chunk.NewLines = append(chunk.NewLines, "")
			parsedLines++
			continue
		}

		marker := line[0]

		if marker == ' ' {
			content := line[1:]
			chunk.OldLines = append(chunk.OldLines, content)
			chunk.NewLines = append(chunk.NewLines, content)
			parsedLines++
			continue
		}
		if marker == '+' {
			chunk.NewLines = append(chunk.NewLines, line[1:])
			parsedLines++
			continue
		}
		if marker == '-' {
			chunk.OldLines = append(chunk.OldLines, line[1:])
			parsedLines++
			continue
		}

		if parsedLines == 0 {
			return UpdateFileChunk{}, 0, fmt.Errorf("invalid patch hunk at line %d: Unexpected line found in update hunk: '%s'. Every line should start with ' ' (context line), '+' (added line), or '-' (removed line)", lineNumber+1, line)
		}
		break
	}

	return chunk, parsedLines + startIndex, nil
}

// ----------------------------------------------------------------------------
// Applying Updates to Files
// ----------------------------------------------------------------------------

func applyUpdateHunk(filePath string, chunks []UpdateFileChunk) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file to update %s: %w", filePath, err)
	}

	originalContents := string(content)
	// Normalize CRLF
	originalContents = strings.ReplaceAll(originalContents, "\r\n", "\n")
	originalLines := strings.Split(originalContents, "\n")

	// Remove trailing empty line if it exists (Split behavior)
	if len(originalLines) > 0 && originalLines[len(originalLines)-1] == "" {
		originalLines = originalLines[:len(originalLines)-1]
	}

	type replacement struct {
		StartIndex int
		OldLen     int
		NewLines   []string
	}

	replacements := []replacement{}
	lineIndex := 0

	for _, chunk := range chunks {
		if chunk.ChangeContext != "" {
			ctxIndex := seekSequence(originalLines, []string{chunk.ChangeContext}, lineIndex, false)
			if ctxIndex == -1 {
				return "", fmt.Errorf("failed to find context '%s' in %s", chunk.ChangeContext, filePath)
			}
			lineIndex = ctxIndex + 1
		}

		if len(chunk.OldLines) == 0 {
			insertionIndex := len(originalLines)
			if len(originalLines) > 0 && originalLines[len(originalLines)-1] == "" {
				insertionIndex = len(originalLines) - 1
			}
			replacements = append(replacements, replacement{
				StartIndex: insertionIndex,
				OldLen:     0,
				NewLines:   chunk.NewLines,
			})
			continue
		}

		pattern := chunk.OldLines
		newSlice := chunk.NewLines
		found := seekSequence(originalLines, pattern, lineIndex, chunk.IsEndOfFile)

		if found == -1 && pattern[len(pattern)-1] == "" {
			pattern = pattern[:len(pattern)-1]
			if len(newSlice) > 0 && newSlice[len(newSlice)-1] == "" {
				newSlice = newSlice[:len(newSlice)-1]
			}
			found = seekSequence(originalLines, pattern, lineIndex, chunk.IsEndOfFile)
		}

		if found == -1 {
			return "", fmt.Errorf("failed to find expected lines in %s:\n%s", filePath, strings.Join(chunk.OldLines, "\n"))
		}

		replacements = append(replacements, replacement{
			StartIndex: found,
			OldLen:     len(pattern),
			NewLines:   newSlice,
		})
		lineIndex = found + len(pattern)
	}

	// Sort replacements by start index
	for i := 0; i < len(replacements); i++ {
		for j := i + 1; j < len(replacements); j++ {
			if replacements[i].StartIndex > replacements[j].StartIndex {
				replacements[i], replacements[j] = replacements[j], replacements[i]
			}
		}
	}

	// Apply replacements backwards to not mess up indices
	result := make([]string, len(originalLines))
	copy(result, originalLines)

	for i := len(replacements) - 1; i >= 0; i-- {
		repl := replacements[i]

		// Delete old lines
		if repl.StartIndex < len(result) {
			end := repl.StartIndex + repl.OldLen
			if end > len(result) {
				end = len(result)
			}
			result = append(result[:repl.StartIndex], result[end:]...)
		}

		// Insert new lines
		// Insert space
		result = append(result[:repl.StartIndex], append(make([]string, len(repl.NewLines)), result[repl.StartIndex:]...)...)
		// Copy elements
		copy(result[repl.StartIndex:], repl.NewLines)
	}

	if len(result) == 0 || result[len(result)-1] != "" {
		result = append(result, "")
	}

	return strings.Join(result, "\n"), nil
}

func seekSequence(lines []string, pattern []string, start int, eof bool) int {
	if len(pattern) == 0 {
		return start
	}
	if len(pattern) > len(lines) {
		return -1
	}

	maxStart := len(lines) - len(pattern)
	searchStart := start
	if eof && len(lines) >= len(pattern) {
		searchStart = maxStart
	}

	if searchStart > maxStart {
		return -1
	}

	// Exact match
	for i := searchStart; i <= maxStart; i++ {
		if linesMatch(lines, pattern, i, func(v string) string { return v }) {
			return i
		}
	}

	// Trim End match
	for i := searchStart; i <= maxStart; i++ {
		if linesMatch(lines, pattern, i, stringsTrimRightSpace) {
			return i
		}
	}

	// Trim both match
	for i := searchStart; i <= maxStart; i++ {
		if linesMatch(lines, pattern, i, strings.TrimSpace) {
			return i
		}
	}

	// Normalize punctuation match
	for i := searchStart; i <= maxStart; i++ {
		if linesMatch(lines, pattern, i, func(v string) string { return normalizePunctuation(strings.TrimSpace(v)) }) {
			return i
		}
	}

	return -1
}

func stringsTrimRightSpace(s string) string {
	return strings.TrimRight(s, " \t\r\n")
}

func linesMatch(lines []string, pattern []string, start int, normalize func(string) string) bool {
	for idx := 0; idx < len(pattern); idx++ {
		if normalize(lines[start+idx]) != normalize(pattern[idx]) {
			return false
		}
	}
	return true
}

func normalizePunctuation(value string) string {
	var sb strings.Builder
	for _, char := range value {
		switch char {
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
			sb.WriteRune('-')
		case '\u2018', '\u2019', '\u201A', '\u201B':
			sb.WriteRune('\'')
		case '\u201C', '\u201D', '\u201E', '\u201F':
			sb.WriteRune('"')
		case '\u00A0', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200A', '\u202F', '\u205F', '\u3000':
			sb.WriteRune(' ')
		default:
			sb.WriteRune(char)
		}
	}
	return sb.String()
}
