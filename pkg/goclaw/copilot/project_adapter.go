// Package copilot – project_adapter.go provides an adapter that bridges
// copilot.ProjectManager to skills.ProjectProvider, breaking the import cycle
// between the two packages.
//
// The adapter converts between copilot.Project and skills.ProjectInfo structs
// which have the same shape but live in different packages.
package copilot

import (
	"github.com/jholhewres/goclaw/pkg/goclaw/skills"
)

// ProjectProviderAdapter adapts a copilot.ProjectManager to the
// skills.ProjectProvider interface so coding skills can access project
// context without importing the copilot package.
type ProjectProviderAdapter struct {
	pm *ProjectManager
}

// NewProjectProviderAdapter creates a new adapter wrapping the given ProjectManager.
func NewProjectProviderAdapter(pm *ProjectManager) *ProjectProviderAdapter {
	return &ProjectProviderAdapter{pm: pm}
}

// Register adds or updates a project.
func (a *ProjectProviderAdapter) Register(p *skills.ProjectInfo) error {
	return a.pm.Register(toProject(p))
}

// Remove removes a project by ID.
func (a *ProjectProviderAdapter) Remove(id string) error {
	return a.pm.Remove(id)
}

// Get returns a project by ID.
func (a *ProjectProviderAdapter) Get(id string) *skills.ProjectInfo {
	return toProjectInfo(a.pm.Get(id))
}

// List returns all registered projects.
func (a *ProjectProviderAdapter) List() []*skills.ProjectInfo {
	projects := a.pm.List()
	result := make([]*skills.ProjectInfo, 0, len(projects))
	for _, p := range projects {
		result = append(result, toProjectInfo(p))
	}
	return result
}

// Activate sets the active project for a session.
func (a *ProjectProviderAdapter) Activate(sessionKey, projectID string) error {
	return a.pm.Activate(sessionKey, projectID)
}

// ActiveProject returns the active project for a session.
func (a *ProjectProviderAdapter) ActiveProject(sessionKey string) *skills.ProjectInfo {
	return toProjectInfo(a.pm.ActiveProject(sessionKey))
}

// ScanDirectory scans for projects in a directory.
func (a *ProjectProviderAdapter) ScanDirectory(root string) ([]*skills.ProjectInfo, error) {
	projects, err := a.pm.ScanDirectory(root)
	if err != nil {
		return nil, err
	}
	result := make([]*skills.ProjectInfo, 0, len(projects))
	for _, p := range projects {
		result = append(result, toProjectInfo(p))
	}
	return result, nil
}

// ── Conversion helpers ──

func toProjectInfo(p *Project) *skills.ProjectInfo {
	if p == nil {
		return nil
	}
	return &skills.ProjectInfo{
		ID:            p.ID,
		Name:          p.Name,
		RootPath:      p.RootPath,
		Language:       p.Language,
		Framework:     p.Framework,
		GitRemote:     p.GitRemote,
		BuildCmd:      p.BuildCmd,
		TestCmd:       p.TestCmd,
		LintCmd:       p.LintCmd,
		StartCmd:      p.StartCmd,
		DeployCmd:     p.DeployCmd,
		DockerCompose: p.DockerCompose,
	}
}

func toProject(p *skills.ProjectInfo) *Project {
	if p == nil {
		return nil
	}
	return &Project{
		ID:            p.ID,
		Name:          p.Name,
		RootPath:      p.RootPath,
		Language:       p.Language,
		Framework:     p.Framework,
		GitRemote:     p.GitRemote,
		BuildCmd:      p.BuildCmd,
		TestCmd:       p.TestCmd,
		LintCmd:       p.LintCmd,
		StartCmd:      p.StartCmd,
		DeployCmd:     p.DeployCmd,
		DockerCompose: p.DockerCompose,
	}
}
