// Package copilot – browser_snapshot.go implements accessibility tree snapshots
// and role reference generation for browser automation.
//
// The accessibility tree provides a structured representation of the page
// that's easier for AI agents to understand and interact with. Role references
// (e1, e2, ...) provide stable element identifiers across multiple snapshots.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// AXRole is a custom type that can unmarshal from either a string or an object.
// Chrome CDP sometimes returns role as {"type": "role", "value": "button"}
// and sometimes as just "button".
type AXRole struct {
	Value string
}

// UnmarshalJSON implements json.Unmarshaler for AXRole.
func (r *AXRole) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		r.Value = str
		return nil
	}

	// Try object with "value" field
	var obj struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		r.Value = obj.Value
		return nil
	}

	// Try object with just value
	var simpleObj map[string]any
	if err := json.Unmarshal(data, &simpleObj); err == nil {
		if v, ok := simpleObj["value"].(string); ok {
			r.Value = v
			return nil
		}
	}

	return fmt.Errorf("cannot unmarshal AXRole from: %s", string(data))
}

// String returns the role value as string.
func (r AXRole) String() string {
	return r.Value
}

// AXValue is a custom type that can unmarshal from either a string or an object.
// Chrome CDP sometimes returns name/value as {"type": "string", "value": "text"}
// and sometimes as just "text".
type AXValue struct {
	Value string
}

// UnmarshalJSON implements json.Unmarshaler for AXValue.
func (v *AXValue) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		v.Value = str
		return nil
	}

	// Try object with "value" field
	var obj struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		v.Value = obj.Value
		return nil
	}

	// Try object with just value
	var simpleObj map[string]any
	if err := json.Unmarshal(data, &simpleObj); err == nil {
		if val, ok := simpleObj["value"].(string); ok {
			v.Value = val
			return nil
		}
	}

	return fmt.Errorf("cannot unmarshal AXValue from: %s", string(data))
}

// String returns the value as string.
func (v AXValue) String() string {
	return v.Value
}

// AXNode represents a node in the accessibility tree from CDP.
type AXNode struct {
	NodeID      string    `json:"nodeId"`
	Role        AXRole    `json:"role"`
	Name        AXValue   `json:"name,omitempty"`
	Value       AXValue   `json:"value,omitempty"`
	Description AXValue   `json:"description,omitempty"`
	Properties  any       `json:"properties,omitempty"`
	Children    []*AXNode `json:"children,omitempty"`

	// Ref is the generated reference (e1, e2, ...) for interactive elements.
	Ref string `json:"ref,omitempty"`
}

// SnapshotResult is the result of a browser snapshot operation.
type SnapshotResult struct {
	// Snapshot is the text representation of the accessibility tree.
	Snapshot string `json:"snapshot"`

	// Refs maps reference IDs (e1, e2, ...) to their element info.
	Refs map[string]Ref `json:"refs"`

	// Stats contains statistics about the snapshot.
	Stats SnapshotStats `json:"stats"`
}

// Ref represents a reference to an element in the page.
type Ref struct {
	Role string `json:"role"`
	Name string `json:"name,omitempty"`
	Nth  int    `json:"nth,omitempty"` // For duplicate role+name combinations
}

// SnapshotStats contains statistics about a snapshot.
type SnapshotStats struct {
	Lines       int `json:"lines"`
	Chars       int `json:"chars"`
	Refs        int `json:"refs"`
	Interactive int `json:"interactive"`
}

// SnapshotOptions configures the snapshot behavior.
type SnapshotOptions struct {
	// InteractiveOnly only includes interactive elements (buttons, links, inputs, etc.).
	InteractiveOnly bool

	// Compact removes structural noise (generic containers without names).
	Compact bool

	// MaxDepth limits the tree depth (0 = unlimited).
	MaxDepth int
}

// InteractiveRoles are roles that represent interactive elements.
var InteractiveRoles = map[string]bool{
	"button":          true,
	"link":            true,
	"textbox":         true,
	"checkbox":        true,
	"radio":           true,
	"combobox":        true,
	"menuitem":        true,
	"menuitemcheckbox": true,
	"menuitemradio":   true,
	"tab":             true,
	"spinbutton":      true,
	"slider":          true,
	"switch":          true,
	"searchbox":       true,
	"textarea":        true,
}

// ContentRoles are roles that represent content elements.
var ContentRoles = map[string]bool{
	"heading":    true,
	"paragraph":  true,
	"cell":       true,
	"rowheader":  true,
	"columnheader": true,
	"listitem":   true,
	"article":    true,
	"figure":     true,
	"img":        true,
}

// StructuralRoles are roles that represent structural elements.
var StructuralRoles = map[string]bool{
	"generic":     true,
	"group":       true,
	"list":        true,
	"table":       true,
	"row":         true,
	"region":      true,
	"section":     true,
	"document":    true,
	"webarea":     true,
	"rootWebArea": true,
}

// GetAccessibilityTree fetches the full accessibility tree via CDP.
func (bm *BrowserManager) GetAccessibilityTree(ctx context.Context) (*AXNode, error) {
	if err := bm.Start(ctx); err != nil {
		return nil, err
	}

	result, err := bm.sendCDP("Accessibility.getFullAXTree", map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("failed to get accessibility tree: %w", err)
	}

	// Parse the CDP response
	var axResult struct {
		Nodes []struct {
			NodeID        string   `json:"nodeId"`
			Role          AXRole   `json:"role"`
			Name          AXValue  `json:"name,omitempty"`
			Value         AXValue  `json:"value,omitempty"`
			Description   AXValue  `json:"description,omitempty"`
			Properties    any      `json:"properties,omitempty"`
			ChildIDs      []string `json:"childIds,omitempty"`
			ParentID      string   `json:"parentId,omitempty"`
		} `json:"nodes"`
	}

	if err := parseJSONResult(result, &axResult); err != nil {
		return nil, fmt.Errorf("failed to parse accessibility tree: %w", err)
	}

	// Build the tree structure
	return bm.buildAXTree(axResult.Nodes)
}

// buildAXTree converts the flat CDP node list into a tree structure.
func (bm *BrowserManager) buildAXTree(nodes []struct {
	NodeID      string   `json:"nodeId"`
	Role        AXRole   `json:"role"`
	Name        AXValue  `json:"name,omitempty"`
	Value       AXValue  `json:"value,omitempty"`
	Description AXValue  `json:"description,omitempty"`
	Properties  any      `json:"properties,omitempty"`
	ChildIDs    []string `json:"childIds,omitempty"`
	ParentID    string   `json:"parentId,omitempty"`
}) (*AXNode, error) {
	// Create a map for quick lookup
	nodeMap := make(map[string]*AXNode)
	for _, n := range nodes {
		nodeMap[n.NodeID] = &AXNode{
			NodeID:      n.NodeID,
			Role:        n.Role,
			Name:        n.Name,
			Value:       n.Value,
			Description: n.Description,
			Properties:  n.Properties,
			Children:    make([]*AXNode, 0),
		}
	}

	// Build tree relationships and find root
	var root *AXNode
	for _, n := range nodes {
		node := nodeMap[n.NodeID]
		if n.ParentID == "" {
			root = node
		} else if parent, ok := nodeMap[n.ParentID]; ok {
			parent.Children = append(parent.Children, node)
		}
	}

	if root == nil && len(nodes) > 0 {
		// Fallback: use first node as root
		root = nodeMap[nodes[0].NodeID]
	}

	return root, nil
}

// Snapshot creates a text snapshot of the current page with role references.
func (bm *BrowserManager) Snapshot(ctx context.Context, opts SnapshotOptions) (*SnapshotResult, error) {
	axTree, err := bm.GetAccessibilityTree(ctx)
	if err != nil {
		return nil, err
	}

	// Generate role references
	refs := buildRoleRefs(axTree, opts.InteractiveOnly)

	// Build the text snapshot
	var builder strings.Builder
	stats := SnapshotStats{}

	bm.renderSnapshot(&builder, axTree, refs, opts, 0, &stats)

	result := &SnapshotResult{
		Snapshot: builder.String(),
		Refs:     refs,
		Stats:    stats,
	}

	// Store refs for this target
	bm.storeRoleRefs(result)

	return result, nil
}

// renderSnapshot renders the accessibility tree as text.
func (bm *BrowserManager) renderSnapshot(builder *strings.Builder, node *AXNode, refs map[string]Ref, opts SnapshotOptions, depth int, stats *SnapshotStats) {
	if node == nil {
		return
	}

	// Check max depth
	if opts.MaxDepth > 0 && depth > opts.MaxDepth {
		return
	}

	// Skip structural elements in compact mode
	if opts.Compact && StructuralRoles[node.Role.Value] && node.Name.Value == "" && len(node.Children) == 0 {
		return
	}

	// Check if interactive-only filter applies
	isInteractive := InteractiveRoles[node.Role.Value]
	if opts.InteractiveOnly && !isInteractive && !ContentRoles[node.Role.Value] {
		// Still process children for interactive elements
		for _, child := range node.Children {
			bm.renderSnapshot(builder, child, refs, opts, depth, stats)
		}
		return
	}

	// Render this node
	indent := strings.Repeat("  ", depth)
	ref := ""
	if node.Ref != "" {
		ref = fmt.Sprintf(" [%s]", node.Ref)
	}

	name := node.Name.Value
	if name != "" && len(name) > 100 {
		name = name[:97] + "..."
	}

	var line string
	if name != "" {
		line = fmt.Sprintf("%s- %s%s: %s\n", indent, node.Role.Value, ref, name)
	} else {
		line = fmt.Sprintf("%s- %s%s\n", indent, node.Role.Value, ref)
	}

	builder.WriteString(line)
	stats.Lines++
	stats.Chars += len(line)

	if isInteractive {
		stats.Interactive++
	}

	// Render children
	for _, child := range node.Children {
		bm.renderSnapshot(builder, child, refs, opts, depth+1, stats)
	}
}

// buildRoleRefs processes the accessibility tree and generates role references.
func buildRoleRefs(root *AXNode, interactiveOnly bool) map[string]Ref {
	refs := make(map[string]Ref)
	counter := 0

	// Track duplicates for nth indexing
	seen := make(map[string]int)

	var process func(node *AXNode)
	process = func(node *AXNode) {
		if node == nil {
			return
		}

		// Only generate refs for interactive elements
		if InteractiveRoles[node.Role.Value] {
			counter++
			refID := fmt.Sprintf("e%d", counter)
			node.Ref = refID

			// Handle duplicates
			key := node.Role.Value + ":" + node.Name.Value
			nth := 0
			if seen[key] > 0 {
				nth = seen[key] + 1
			}
			seen[key]++

			refs[refID] = Ref{
				Role: node.Role.Value,
				Name: node.Name.Value,
				Nth:  nth,
			}
		}

		for _, child := range node.Children {
			process(child)
		}
	}

	process(root)
	return refs
}

// storeRoleRefs stores the role references for later use in actions.
func (bm *BrowserManager) storeRoleRefs(result *SnapshotResult) {
	bm.roleRefsMu.Lock()
	defer bm.roleRefsMu.Unlock()

	// Use "default" as targetId for now (single tab mode)
	if bm.roleRefs == nil {
		bm.roleRefs = make(map[string]map[string]Ref)
	}
	bm.roleRefs["default"] = result.Refs

	// Update page state
	bm.pageStateMu.Lock()
	defer bm.pageStateMu.Unlock()
	if bm.pageState == nil {
		bm.pageState = make(map[string]*PageState)
	}
	if bm.pageState["default"] == nil {
		bm.pageState["default"] = &PageState{}
	}
	bm.pageState["default"].LastSnapshot = result
}

// getRoleRef retrieves a role reference by ID.
func (bm *BrowserManager) getRoleRef(refID string) (Ref, bool) {
	bm.roleRefsMu.RLock()
	defer bm.roleRefsMu.RUnlock()

	if bm.roleRefs == nil {
		return Ref{}, false
	}

	refs, ok := bm.roleRefs["default"]
	if !ok {
		return Ref{}, false
	}

	ref, ok := refs[refID]
	return ref, ok
}

// ResolveRefToSelector converts a role reference to a CSS selector or JS query.
func (bm *BrowserManager) ResolveRefToSelector(refID string) (string, error) {
	ref, ok := bm.getRoleRef(refID)
	if !ok {
		return "", fmt.Errorf("reference not found: %s", refID)
	}

	// Build selector based on role and name
	selector := buildSelectorFromRef(ref)
	return selector, nil
}

// buildSelectorFromRef creates a CSS selector from a Ref.
func buildSelectorFromRef(ref Ref) string {
	// Map ARIA roles to CSS selectors
	switch ref.Role {
	case "button":
		if ref.Name != "" {
			return fmt.Sprintf(`button:has-text("%s"), input[type="button"][value="%s"], [role="button"]:has-text("%s")`, ref.Name, ref.Name, ref.Name)
		}
		return "button, [role='button']"

	case "link":
		if ref.Name != "" {
			return fmt.Sprintf(`a:has-text("%s"), [role="link"]:has-text("%s")`, ref.Name, ref.Name)
		}
		return "a, [role='link']"

	case "textbox", "searchbox", "textarea":
		if ref.Name != "" {
			return fmt.Sprintf(`input[aria-label="%s"], input[placeholder*="%s"], textarea[aria-label="%s"], [role="textbox"][aria-label="%s"]`, ref.Name, ref.Name, ref.Name, ref.Name)
		}
		return "input:not([type]), input[type='text'], input[type='search'], textarea, [role='textbox']"

	case "checkbox":
		if ref.Name != "" {
			return fmt.Sprintf(`input[type="checkbox"][aria-label="%s"], input[type="checkbox"] ~ label:has-text("%s")`, ref.Name, ref.Name)
		}
		return "input[type='checkbox'], [role='checkbox']"

	case "radio":
		if ref.Name != "" {
			return fmt.Sprintf(`input[type="radio"][aria-label="%s"], input[type="radio"] ~ label:has-text("%s")`, ref.Name, ref.Name)
		}
		return "input[type='radio'], [role='radio']"

	case "combobox", "select":
		if ref.Name != "" {
			return fmt.Sprintf(`select[aria-label="%s"], [role="combobox"][aria-label="%s"]`, ref.Name, ref.Name)
		}
		return "select, [role='combobox']"

	default:
		if ref.Name != "" {
			return fmt.Sprintf(`[role="%s"]:has-text("%s")`, ref.Role, ref.Name)
		}
		return fmt.Sprintf(`[role="%s"]`, ref.Role)
	}
}

// parseJSONResult is a helper to parse CDP JSON results.
func parseJSONResult(data json.RawMessage, v any) error {
	return json.Unmarshal(data, v)
}

// PageState tracks per-page state.
type PageState struct {
	ConsoleMessages []ConsoleMessage
	NetworkRequests []NetworkRequest
	LastSnapshot    *SnapshotResult
}

// ConsoleMessage represents a browser console message.
type ConsoleMessage struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Level   string `json:"level"`
	URL     string `json:"url,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}

// NetworkRequest represents a network request.
type NetworkRequest struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Status    int               `json:"status"`
	Type      string            `json:"type"`
	Headers   map[string]string `json:"headers,omitempty"`
	Timestamp int64             `json:"timestamp"`
}

// SortedRefs returns refs sorted by their numeric ID.
func (s *SnapshotResult) SortedRefs() []string {
	ids := make([]string, 0, len(s.Refs))
	for id := range s.Refs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		// Sort e1, e2, e3, ... numerically
		var num1, num2 int
		fmt.Sscanf(ids[i], "e%d", &num1)
		fmt.Sscanf(ids[j], "e%d", &num2)
		return num1 < num2
	})
	return ids
}
