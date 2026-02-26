// Package copilot – browser_tabs.go implements tab management tools
// for browser automation.
package copilot

import (
	"context"
	"fmt"
)

// Tab represents a browser tab.
type Tab struct {
	TargetID string `json:"targetId"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Type     string `json:"type"`
}

// ListTabs returns all open browser tabs.
func (bm *BrowserManager) ListTabs(ctx context.Context) ([]Tab, error) {
	if err := bm.Start(ctx); err != nil {
		return nil, err
	}

	result, err := bm.sendCDP("Target.getTargets", map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("failed to list targets: %w", err)
	}

	var targetsResult struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
			Title    string `json:"title"`
			URL      string `json:"url"`
			Attached bool   `json:"attached"`
		} `json:"targetInfos"`
	}

	if err := parseJSONResult(result, &targetsResult); err != nil {
		return nil, fmt.Errorf("failed to parse targets: %w", err)
	}

	// Filter to only page types
	tabs := make([]Tab, 0)
	for _, info := range targetsResult.TargetInfos {
		if info.Type == "page" {
			tabs = append(tabs, Tab{
				TargetID: info.TargetID,
				URL:      info.URL,
				Title:    info.Title,
				Type:     info.Type,
			})
		}
	}

	return tabs, nil
}

// OpenTab opens a new browser tab and optionally navigates to a URL.
func (bm *BrowserManager) OpenTab(ctx context.Context, url string) (*Tab, error) {
	if err := bm.Start(ctx); err != nil {
		return nil, err
	}

	// Create new target
	result, err := bm.sendCDP("Target.createTarget", map[string]any{
		"url": url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create target: %w", err)
	}

	var createResult struct {
		TargetID string `json:"targetId"`
	}

	if err := parseJSONResult(result, &createResult); err != nil {
		return nil, fmt.Errorf("failed to parse create target result: %w", err)
	}

	return &Tab{
		TargetID: createResult.TargetID,
		URL:      url,
		Type:     "page",
	}, nil
}

// FocusTab focuses a browser tab by target ID.
func (bm *BrowserManager) FocusTab(ctx context.Context, targetID string) error {
	if err := bm.Start(ctx); err != nil {
		return err
	}

	// Activate the target
	_, err := bm.sendCDP("Target.activateTarget", map[string]any{
		"targetId": targetID,
	})
	if err != nil {
		return fmt.Errorf("failed to activate target: %w", err)
	}

	return nil
}

// CloseTab closes a browser tab by target ID.
func (bm *BrowserManager) CloseTab(ctx context.Context, targetID string) error {
	if err := bm.Start(ctx); err != nil {
		return err
	}

	// Close the target
	_, err := bm.sendCDP("Target.closeTarget", map[string]any{
		"targetId": targetID,
	})
	if err != nil {
		return fmt.Errorf("failed to close target: %w", err)
	}

	return nil
}

// GetCurrentTab returns the currently active tab.
func (bm *BrowserManager) GetCurrentTab(ctx context.Context) (*Tab, error) {
	tabs, err := bm.ListTabs(ctx)
	if err != nil {
		return nil, err
	}

	if len(tabs) == 0 {
		return nil, fmt.Errorf("no tabs open")
	}

	// Return first tab (CDP doesn't have a direct way to get active tab)
	// In practice, the first tab in the list is usually the active one
	return &tabs[0], nil
}

// NavigateTab navigates a specific tab to a URL.
func (bm *BrowserManager) NavigateTab(ctx context.Context, targetID, url string) error {
	if err := bm.Start(ctx); err != nil {
		return err
	}

	// SSRF check
	if bm.ssrfGuard != nil {
		if err := bm.ssrfGuard.IsAllowed(url); err != nil {
			return fmt.Errorf("browser navigation blocked: %w", err)
		}
	}

	_, err := bm.sendCDP("Page.navigate", map[string]any{
		"frameId":  targetID,
		"url":      url,
	})
	if err != nil {
		return fmt.Errorf("failed to navigate: %w", err)
	}

	return nil
}
