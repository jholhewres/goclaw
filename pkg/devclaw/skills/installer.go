// Package skills – installer.go implements skill installation from multiple
// sources: ClawHub registry, GitHub repositories, URLs, and local paths.
//
// Supported sources:
//   - ClawHub slug:     "steipete/trello" or "clawhub:steipete/trello"
//   - ClawHub URL:      "https://clawhub.ai/steipete/trello"
//   - GitHub URL:       "https://github.com/user/repo" or "github:user/repo"
//   - HTTP URL:         "https://example.com/skill.zip" or raw SKILL.md URL
//   - Local path:       "./my-skill" or "/home/user/skills/my-skill"
package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Installer handles skill installation from various sources.
type Installer struct {
	skillsDir string // target directory for installed skills
	clawhub   *ClawHubClient
	logger    *slog.Logger
}

// InstallResult holds the result of a skill installation.
type InstallResult struct {
	Name    string // skill name (directory name)
	Source  string // where it came from
	Path    string // full path to the installed skill
	IsNew   bool   // true if newly installed, false if updated
	Version string // version if known
}

// NewInstaller creates a new skill installer.
func NewInstaller(skillsDir string, logger *slog.Logger) *Installer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Installer{
		skillsDir: skillsDir,
		clawhub:   NewClawHubClient(""),
		logger:    logger.With("component", "skill_installer"),
	}
}

// Install installs a skill from the given source string.
// It auto-detects the source type based on the input format.
func (inst *Installer) Install(ctx context.Context, source string) (*InstallResult, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("empty skill source")
	}

	// Ensure skills directory exists.
	if err := os.MkdirAll(inst.skillsDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating skills directory: %w", err)
	}

	// Detect source type and install.
	switch {
	case strings.HasPrefix(source, "clawhub:"):
		slug := strings.TrimPrefix(source, "clawhub:")
		return inst.installFromClawHub(ctx, slug)

	case strings.HasPrefix(source, "github:"):
		repo := strings.TrimPrefix(source, "github:")
		return inst.installFromGitHub(ctx, repo)

	case strings.HasPrefix(source, "https://clawhub.ai/") || strings.HasPrefix(source, "https://clawhub.com/"):
		slug := extractClawHubSlug(source)
		if slug == "" {
			return nil, fmt.Errorf("invalid ClawHub URL: %s", source)
		}
		return inst.installFromClawHub(ctx, slug)

	case strings.HasPrefix(source, "https://github.com/") || strings.HasPrefix(source, "http://github.com/"):
		repo := extractGitHubRepo(source)
		if repo == "" {
			return nil, fmt.Errorf("invalid GitHub URL: %s", source)
		}
		return inst.installFromGitHub(ctx, repo)

	case strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://"):
		return inst.installFromURL(ctx, source)

	case isLocalPath(source):
		return inst.installFromLocal(source)

	default:
		// Try as ClawHub slug (e.g. "steipete/trello" or just "trello").
		if strings.Contains(source, "/") || !strings.Contains(source, ".") {
			result, err := inst.installFromClawHub(ctx, source)
			if err == nil {
				return result, nil
			}
			inst.logger.Debug("ClawHub lookup failed, trying other sources", "error", err)
		}
		return nil, fmt.Errorf("cannot determine source type for %q. Use clawhub:<slug>, github:<user/repo>, a URL, or a local path", source)
	}
}

// installFromClawHub installs a skill from the ClawHub registry.
func (inst *Installer) installFromClawHub(ctx context.Context, slug string) (*InstallResult, error) {
	inst.logger.Info("installing from ClawHub", "slug", slug)

	// Try downloading the skill archive.
	data, err := inst.clawhub.Download(slug, "")
	if err != nil {
		// Fallback: try fetching just the SKILL.md.
		inst.logger.Debug("archive download failed, trying SKILL.md", "error", err)
		return inst.installClawHubSkillMD(ctx, slug)
	}

	// Extract zip archive.
	name := skillNameFromSlug(slug)
	targetDir := filepath.Join(inst.skillsDir, name)
	isNew := !dirExists(targetDir)

	if err := extractZip(data, targetDir); err != nil {
		return nil, fmt.Errorf("extracting skill archive: %w", err)
	}

	inst.logger.Info("skill installed from ClawHub", "name", name, "path", targetDir)
	return &InstallResult{
		Name:   name,
		Source: "clawhub:" + slug,
		Path:   targetDir,
		IsNew:  isNew,
	}, nil
}

// installClawHubSkillMD fetches just the SKILL.md and creates the skill directory.
func (inst *Installer) installClawHubSkillMD(_ context.Context, slug string) (*InstallResult, error) {
	content, err := inst.clawhub.FetchFile(slug, "SKILL.md")
	if err != nil {
		return nil, fmt.Errorf("fetching SKILL.md from ClawHub: %w", err)
	}

	name := skillNameFromSlug(slug)
	targetDir := filepath.Join(inst.skillsDir, name)
	isNew := !dirExists(targetDir)

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "SKILL.md"), content, 0o644); err != nil {
		return nil, err
	}

	// Also create scripts directory.
	_ = os.MkdirAll(filepath.Join(targetDir, "scripts"), 0o755)

	inst.logger.Info("skill installed from ClawHub (SKILL.md)", "name", name)
	return &InstallResult{
		Name:   name,
		Source: "clawhub:" + slug,
		Path:   targetDir,
		IsNew:  isNew,
	}, nil
}

// installFromGitHub clones a GitHub repository into the skills directory.
func (inst *Installer) installFromGitHub(ctx context.Context, repo string) (*InstallResult, error) {
	inst.logger.Info("installing from GitHub", "repo", repo)

	// Determine skill name from repo.
	parts := strings.Split(repo, "/")
	name := parts[len(parts)-1]
	name = strings.TrimSuffix(name, ".git")

	// Handle sub-paths (e.g. "user/repo/path/to/skill").
	var subPath string
	if len(parts) > 2 {
		repo = parts[0] + "/" + parts[1]
		subPath = strings.Join(parts[2:], "/")
		name = parts[len(parts)-1]
	}

	cloneURL := fmt.Sprintf("https://github.com/%s.git", repo)
	targetDir := filepath.Join(inst.skillsDir, name)
	isNew := !dirExists(targetDir)

	if subPath != "" {
		// Clone to temp, copy the sub-path.
		return inst.installGitHubSubPath(ctx, cloneURL, subPath, name, targetDir, isNew)
	}

	// Full clone (or pull if exists).
	if dirExists(targetDir) {
		// Update: git pull.
		cmd := exec.CommandContext(ctx, "git", "-C", targetDir, "pull", "--ff-only")
		if out, err := cmd.CombinedOutput(); err != nil {
			inst.logger.Warn("git pull failed", "output", string(out), "error", err)
			// Try fresh clone.
			_ = os.RemoveAll(targetDir)
			isNew = true
		} else {
			inst.logger.Info("skill updated from GitHub", "name", name)
			return &InstallResult{Name: name, Source: "github:" + repo, Path: targetDir, IsNew: false}, nil
		}
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", cloneURL, targetDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git clone failed: %s: %w", string(out), err)
	}

	inst.logger.Info("skill installed from GitHub", "name", name, "path", targetDir)
	return &InstallResult{
		Name:   name,
		Source: "github:" + repo,
		Path:   targetDir,
		IsNew:  isNew,
	}, nil
}

// installGitHubSubPath clones a repo and extracts a sub-path.
func (inst *Installer) installGitHubSubPath(ctx context.Context, cloneURL, subPath, name, targetDir string, isNew bool) (*InstallResult, error) {
	tmpDir, err := os.MkdirTemp("", "devclaw-skill-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", cloneURL, tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git clone failed: %s: %w", string(out), err)
	}

	srcDir := filepath.Join(tmpDir, subPath)
	if !dirExists(srcDir) {
		return nil, fmt.Errorf("sub-path %q not found in repository", subPath)
	}

	// Copy to target.
	_ = os.RemoveAll(targetDir)
	if err := copyDir(srcDir, targetDir); err != nil {
		return nil, fmt.Errorf("copying skill: %w", err)
	}

	return &InstallResult{
		Name:   name,
		Source: "github:" + cloneURL + "/" + subPath,
		Path:   targetDir,
		IsNew:  isNew,
	}, nil
}

// installFromURL downloads a skill from a URL (zip archive or raw SKILL.md).
func (inst *Installer) installFromURL(ctx context.Context, rawURL string) (*InstallResult, error) {
	inst.logger.Info("installing from URL", "url", rawURL)

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "DevClaw/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Determine name from URL.
	name := skillNameFromURL(rawURL)
	targetDir := filepath.Join(inst.skillsDir, name)
	isNew := !dirExists(targetDir)

	contentType := resp.Header.Get("Content-Type")

	// If it looks like a zip file.
	if strings.Contains(contentType, "zip") || strings.HasSuffix(rawURL, ".zip") || isZipData(data) {
		if err := extractZip(data, targetDir); err != nil {
			return nil, fmt.Errorf("extracting zip: %w", err)
		}
	} else {
		// Treat as raw SKILL.md content.
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(targetDir, "SKILL.md"), data, 0o644); err != nil {
			return nil, err
		}
		_ = os.MkdirAll(filepath.Join(targetDir, "scripts"), 0o755)
	}

	inst.logger.Info("skill installed from URL", "name", name, "path", targetDir)
	return &InstallResult{
		Name:   name,
		Source: rawURL,
		Path:   targetDir,
		IsNew:  isNew,
	}, nil
}

// installFromLocal copies a local skill directory to the skills dir.
func (inst *Installer) installFromLocal(source string) (*InstallResult, error) {
	inst.logger.Info("installing from local path", "path", source)

	absSource, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}

	if !dirExists(absSource) {
		// Check if it's a SKILL.md file.
		if _, err := os.Stat(absSource); err == nil {
			return inst.installLocalFile(absSource)
		}
		return nil, fmt.Errorf("local path not found: %s", absSource)
	}

	name := filepath.Base(absSource)
	targetDir := filepath.Join(inst.skillsDir, name)

	// Don't copy if source IS the target.
	if absSource == targetDir {
		return &InstallResult{Name: name, Source: absSource, Path: targetDir, IsNew: false}, nil
	}

	isNew := !dirExists(targetDir)
	_ = os.RemoveAll(targetDir)

	if err := copyDir(absSource, targetDir); err != nil {
		return nil, fmt.Errorf("copying skill: %w", err)
	}

	inst.logger.Info("skill installed from local", "name", name, "path", targetDir)
	return &InstallResult{
		Name:   name,
		Source: absSource,
		Path:   targetDir,
		IsNew:  isNew,
	}, nil
}

// installLocalFile installs from a single SKILL.md file.
func (inst *Installer) installLocalFile(filePath string) (*InstallResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	if name == "SKILL" {
		name = filepath.Base(filepath.Dir(filePath))
	}

	targetDir := filepath.Join(inst.skillsDir, name)
	isNew := !dirExists(targetDir)

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "SKILL.md"), data, 0o644); err != nil {
		return nil, err
	}
	_ = os.MkdirAll(filepath.Join(targetDir, "scripts"), 0o755)

	return &InstallResult{Name: name, Source: filePath, Path: targetDir, IsNew: isNew}, nil
}

// ---------- Helpers ----------

// extractClawHubSlug extracts the slug from a ClawHub URL.
// e.g. "https://clawhub.ai/steipete/trello" -> "steipete/trello"
func extractClawHubSlug(u string) string {
	for _, prefix := range []string{"https://clawhub.ai/", "https://clawhub.com/"} {
		if strings.HasPrefix(u, prefix) {
			slug := strings.TrimPrefix(u, prefix)
			slug = strings.TrimSuffix(slug, "/")
			// Remove any query params or fragments.
			if idx := strings.IndexAny(slug, "?#"); idx >= 0 {
				slug = slug[:idx]
			}
			return slug
		}
	}
	return ""
}

// extractGitHubRepo extracts "user/repo" from a GitHub URL.
func extractGitHubRepo(u string) string {
	for _, prefix := range []string{"https://github.com/", "http://github.com/"} {
		if strings.HasPrefix(u, prefix) {
			path := strings.TrimPrefix(u, prefix)
			path = strings.TrimSuffix(path, "/")
			path = strings.TrimSuffix(path, ".git")
			// Remove /tree/main/... or /blob/... suffixes, keep full path for sub-dirs.
			if idx := strings.Index(path, "/tree/"); idx >= 0 {
				branch := path[idx+6:]
				if slashIdx := strings.Index(branch, "/"); slashIdx >= 0 {
					// user/repo/tree/main/sub/path -> user/repo/sub/path
					path = path[:idx] + branch[slashIdx:]
				} else {
					path = path[:idx]
				}
			}
			return path
		}
	}
	return ""
}

// skillNameFromSlug extracts the skill name from a ClawHub slug.
func skillNameFromSlug(slug string) string {
	parts := strings.Split(slug, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return slug
}

// skillNameFromURL extracts a skill name from a URL.
func skillNameFromURL(u string) string {
	base := filepath.Base(u)
	// Remove query params.
	if idx := strings.IndexAny(base, "?#"); idx >= 0 {
		base = base[:idx]
	}
	// Remove extension.
	base = strings.TrimSuffix(base, ".zip")
	base = strings.TrimSuffix(base, ".tar.gz")
	base = strings.TrimSuffix(base, ".md")
	if base == "" || base == "SKILL" {
		base = "downloaded-skill"
	}
	return base
}

// isLocalPath checks if a string looks like a local filesystem path.
func isLocalPath(s string) bool {
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.HasPrefix(s, "~")
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isZipData checks if data starts with the ZIP magic number.
func isZipData(data []byte) bool {
	return len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && data[2] == 0x03 && data[3] == 0x04
}

// extractZip extracts a zip archive to the target directory.
func extractZip(data []byte, targetDir string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}

	// Determine if there's a single root directory to strip.
	stripPrefix := ""
	if len(reader.File) > 0 {
		first := reader.File[0].Name
		if strings.HasSuffix(first, "/") {
			// Check if all files share this prefix.
			allMatch := true
			for _, f := range reader.File[1:] {
				if !strings.HasPrefix(f.Name, first) {
					allMatch = false
					break
				}
			}
			if allMatch {
				stripPrefix = first
			}
		}
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}

	for _, f := range reader.File {
		name := f.Name
		if stripPrefix != "" {
			name = strings.TrimPrefix(name, stripPrefix)
		}
		if name == "" {
			continue
		}

		// Security: reject symlink entries.
		if f.Mode()&os.ModeSymlink != 0 {
			continue
		}

		targetPath, err := safeJoinPath(targetDir, name)
		if err != nil {
			continue // skip path traversal attempts
		}

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(targetPath, 0o755)
			continue
		}

		// Ensure parent directory exists.
		_ = os.MkdirAll(filepath.Dir(targetPath), 0o755)

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		if _, err := io.Copy(outFile, io.LimitReader(rc, 10*1024*1024)); err != nil {
			outFile.Close()
			rc.Close()
			return err
		}

		outFile.Close()
		rc.Close()
	}

	return nil
}

// safeJoinPath joins base and name, returning an error if the result
// escapes base (Zip Slip / path traversal protection).
func safeJoinPath(base, name string) (string, error) {
	p := filepath.Clean(filepath.Join(base, name))
	cleanBase := filepath.Clean(base)
	if p != cleanBase && !strings.HasPrefix(p, cleanBase+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes base %q", name, base)
	}
	return p, nil
}

// copyDir recursively copies a directory, skipping symlinks.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip symlinks — never follow them into the source tree.
		linfo, lerr := os.Lstat(path)
		if lerr != nil {
			return lerr
		}
		if linfo.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		relPath, _ := filepath.Rel(src, path)
		targetPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, linfo.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, linfo.Mode())
	})
}
