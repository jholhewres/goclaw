// Package copilot – browser_act.go implements unified browser actions
// for browser automation.
package copilot

import (
	"context"
	"fmt"
	"time"
)

// Act kinds
const (
	ActClick    = "click"
	ActType     = "type"
	ActPress    = "press"
	ActHover    = "hover"
	ActDrag     = "drag"
	ActSelect   = "select"
	ActFill     = "fill"
	ActResize   = "resize"
	ActWait     = "wait"
	ActEvaluate = "evaluate"
)

// ActRequest represents a browser action request.
type ActRequest struct {
	Kind     string   `json:"kind"`
	Ref      string   `json:"ref,omitempty"`      // Element reference (e1, e2, ...)
	Selector string   `json:"selector,omitempty"` // CSS selector (alternative to ref)

	// For type
	Text string `json:"text,omitempty"`

	// For press
	Key string `json:"key,omitempty"`

	// For drag
	StartRef string `json:"start_ref,omitempty"`
	EndRef   string `json:"end_ref,omitempty"`

	// For select
	Values []string `json:"values,omitempty"`

	// For fill (form fields)
	Fields []FormField `json:"fields,omitempty"`

	// For resize
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`

	// For wait
	TimeMs   int    `json:"time_ms,omitempty"`
	TextGone string `json:"text_gone,omitempty"`

	// For evaluate
	Function string `json:"fn,omitempty"`

	// Modifiers (for click)
	DoubleClick bool     `json:"double_click,omitempty"`
	Button      string   `json:"button,omitempty"` // left, right, middle
	Modifiers   []string `json:"modifiers,omitempty"`

	// For type
	Submit bool `json:"submit,omitempty"` // Press Enter after typing
	Slowly bool `json:"slowly,omitempty"` // Type character by character
}

// FormField represents a form field for fill action.
type FormField struct {
	Ref   string `json:"ref"`
	Type  string `json:"type"`  // textbox, checkbox, radio, combobox
	Value string `json:"value"` // For text inputs
}

// ActResult represents the result of a browser action.
type ActResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// Act performs a browser action.
func (bm *BrowserManager) Act(ctx context.Context, req ActRequest) (*ActResult, error) {
	if err := bm.Start(ctx); err != nil {
		return nil, err
	}

	switch req.Kind {
	case ActClick:
		return bm.actClick(ctx, req)
	case ActType:
		return bm.actType(ctx, req)
	case ActPress:
		return bm.actPress(ctx, req)
	case ActHover:
		return bm.actHover(ctx, req)
	case ActDrag:
		return bm.actDrag(ctx, req)
	case ActSelect:
		return bm.actSelect(ctx, req)
	case ActFill:
		return bm.actFillForm(ctx, req)
	case ActResize:
		return bm.actResize(ctx, req)
	case ActWait:
		return bm.actWait(ctx, req)
	case ActEvaluate:
		return bm.actEvaluate(ctx, req)
	default:
		return nil, fmt.Errorf("unknown action kind: %s", req.Kind)
	}
}

// resolveRefOrSelector returns a selector from either a ref or a direct selector.
func (bm *BrowserManager) resolveRefOrSelector(ref, selector string) (string, error) {
	if ref != "" {
		resolved, err := bm.ResolveRefToSelector(ref)
		if err != nil {
			return "", fmt.Errorf("failed to resolve ref %s: %w", ref, err)
		}
		return resolved, nil
	}
	if selector != "" {
		return selector, nil
	}
	return "", fmt.Errorf("either ref or selector is required")
}

// actClick performs a click action.
func (bm *BrowserManager) actClick(ctx context.Context, req ActRequest) (*ActResult, error) {
	selector, err := bm.resolveRefOrSelector(req.Ref, req.Selector)
	if err != nil {
		return nil, err
	}

	// Build click options
	clickType := "click"
	if req.DoubleClick {
		clickType = "dblclick"
	}

	button := req.Button
	if button == "" {
		button = "left"
	}

	// Build modifiers string
	modifiers := ""
	for _, m := range req.Modifiers {
		switch m {
		case "Alt", "Control", "Shift", "Meta":
			if modifiers != "" {
				modifiers += "+"
			}
			modifiers += m
		}
	}

	// Execute click via JavaScript for more control
	js := fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (!el) return 'not_found';

			// Scroll into view
			el.scrollIntoView({ behavior: 'instant', block: 'center' });

			// Create and dispatch event
			var event;
			if (%q === 'dblclick') {
				event = new MouseEvent('dblclick', { bubbles: true, cancelable: true, button: 0 });
			} else {
				var buttonNum = 0;
				if (%q === 'right') buttonNum = 2;
				else if (%q === 'middle') buttonNum = 1;
				event = new MouseEvent('click', { bubbles: true, cancelable: true, button: buttonNum });
			}
			el.dispatchEvent(event);

			// Also trigger for React/Vue etc.
			el.click();

			return 'ok';
		})()
	`, selector, clickType, button, button)

	result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
		"expression": js,
	})
	if err != nil {
		return nil, fmt.Errorf("click failed: %w", err)
	}

	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := parseJSONResult(result, &evalResult); err != nil {
		return nil, err
	}

	if evalResult.Result.Value == "not_found" {
		return nil, fmt.Errorf("element not found: %s (ref: %s)", selector, req.Ref)
	}

	return &ActResult{OK: true, Message: fmt.Sprintf("Clicked element: %s", req.Ref)}, nil
}

// actType performs a type action.
func (bm *BrowserManager) actType(ctx context.Context, req ActRequest) (*ActResult, error) {
	selector, err := bm.resolveRefOrSelector(req.Ref, req.Selector)
	if err != nil {
		return nil, err
	}

	if req.Text == "" {
		return nil, fmt.Errorf("text is required for type action")
	}

	// Clear and type
	js := fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (!el) return 'not_found';

			// Focus
			el.focus();

			// Clear
			el.value = '';

			// Set value
			el.value = %q;

			// Dispatch events for React/Vue etc.
			el.dispatchEvent(new Event('input', { bubbles: true }));
			el.dispatchEvent(new Event('change', { bubbles: true }));

			return 'ok';
		})()
	`, selector, req.Text)

	result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
		"expression": js,
	})
	if err != nil {
		return nil, fmt.Errorf("type failed: %w", err)
	}

	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := parseJSONResult(result, &evalResult); err != nil {
		return nil, err
	}

	if evalResult.Result.Value == "not_found" {
		return nil, fmt.Errorf("element not found: %s (ref: %s)", selector, req.Ref)
	}

	// Submit if requested
	if req.Submit {
		// Press Enter
		_, _ = bm.sendCDP("Input.dispatchKeyEvent", map[string]any{
			"type": "keyDown",
			"key":  "Enter",
		})
		_, _ = bm.sendCDP("Input.dispatchKeyEvent", map[string]any{
			"type": "keyUp",
			"key":  "Enter",
		})
	}

	return &ActResult{OK: true, Message: fmt.Sprintf("Typed text into element: %s", req.Ref)}, nil
}

// actPress performs a key press action.
func (bm *BrowserManager) actPress(ctx context.Context, req ActRequest) (*ActResult, error) {
	if req.Key == "" {
		return nil, fmt.Errorf("key is required for press action")
	}

	// Dispatch key events
	_, err := bm.sendCDP("Input.dispatchKeyEvent", map[string]any{
		"type": "keyDown",
		"key":  req.Key,
	})
	if err != nil {
		return nil, fmt.Errorf("key press failed: %w", err)
	}

	_, err = bm.sendCDP("Input.dispatchKeyEvent", map[string]any{
		"type": "keyUp",
		"key":  req.Key,
	})
	if err != nil {
		return nil, fmt.Errorf("key release failed: %w", err)
	}

	return &ActResult{OK: true, Message: fmt.Sprintf("Pressed key: %s", req.Key)}, nil
}

// actHover performs a hover action.
func (bm *BrowserManager) actHover(ctx context.Context, req ActRequest) (*ActResult, error) {
	selector, err := bm.resolveRefOrSelector(req.Ref, req.Selector)
	if err != nil {
		return nil, err
	}

	js := fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (!el) return 'not_found';

			// Scroll into view
			el.scrollIntoView({ behavior: 'instant', block: 'center' });

			// Dispatch hover event
			var event = new MouseEvent('mouseover', { bubbles: true, cancelable: true });
			el.dispatchEvent(event);

			return 'ok';
		})()
	`, selector)

	result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
		"expression": js,
	})
	if err != nil {
		return nil, fmt.Errorf("hover failed: %w", err)
	}

	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := parseJSONResult(result, &evalResult); err != nil {
		return nil, err
	}

	if evalResult.Result.Value == "not_found" {
		return nil, fmt.Errorf("element not found: %s (ref: %s)", selector, req.Ref)
	}

	return &ActResult{OK: true, Message: fmt.Sprintf("Hovered over element: %s", req.Ref)}, nil
}

// actDrag performs a drag action.
func (bm *BrowserManager) actDrag(ctx context.Context, req ActRequest) (*ActResult, error) {
	if req.StartRef == "" || req.EndRef == "" {
		return nil, fmt.Errorf("start_ref and end_ref are required for drag action")
	}

	startSelector, err := bm.ResolveRefToSelector(req.StartRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve start_ref: %w", err)
	}

	endSelector, err := bm.ResolveRefToSelector(req.EndRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve end_ref: %w", err)
	}

	// Get element positions and simulate drag
	js := fmt.Sprintf(`
		(function() {
			var startEl = document.querySelector(%q);
			var endEl = document.querySelector(%q);
			if (!startEl || !endEl) return 'not_found';

			// Get positions
			var startRect = startEl.getBoundingClientRect();
			var endRect = endEl.getBoundingClientRect();

			// Simulate drag via dataTransfer (simplified)
			startEl.draggable = true;

			var dragStart = new DragEvent('dragstart', {
				bubbles: true,
				clientX: startRect.left + startRect.width/2,
				clientY: startRect.top + startRect.height/2
			});
			startEl.dispatchEvent(dragStart);

			var drop = new DragEvent('drop', {
				bubbles: true,
				clientX: endRect.left + endRect.width/2,
				clientY: endRect.top + endRect.height/2
			});
			endEl.dispatchEvent(drop);

			return 'ok';
		})()
	`, startSelector, endSelector)

	result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
		"expression": js,
	})
	if err != nil {
		return nil, fmt.Errorf("drag failed: %w", err)
	}

	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := parseJSONResult(result, &evalResult); err != nil {
		return nil, err
	}

	if evalResult.Result.Value == "not_found" {
		return nil, fmt.Errorf("element not found for drag")
	}

	return &ActResult{OK: true, Message: fmt.Sprintf("Dragged from %s to %s", req.StartRef, req.EndRef)}, nil
}

// actSelect performs a select action (for dropdowns).
func (bm *BrowserManager) actSelect(ctx context.Context, req ActRequest) (*ActResult, error) {
	selector, err := bm.resolveRefOrSelector(req.Ref, req.Selector)
	if err != nil {
		return nil, err
	}

	if len(req.Values) == 0 {
		return nil, fmt.Errorf("values are required for select action")
	}

	// Build values array for JS
	valuesJS := "["
	for i, v := range req.Values {
		if i > 0 {
			valuesJS += ", "
		}
		valuesJS += fmt.Sprintf("%q", v)
	}
	valuesJS += "]"

	js := fmt.Sprintf(`
		(function() {
			var el = document.querySelector(%q);
			if (!el) return 'not_found';

			// Clear selection
			for (var i = 0; i < el.options.length; i++) {
				el.options[i].selected = false;
			}

			// Set new selection
			var values = %s;
			for (var i = 0; i < el.options.length; i++) {
				if (values.includes(el.options[i].value)) {
					el.options[i].selected = true;
				}
			}

			el.dispatchEvent(new Event('change', { bubbles: true }));
			return 'ok';
		})()
	`, selector, valuesJS)

	result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
		"expression": js,
	})
	if err != nil {
		return nil, fmt.Errorf("select failed: %w", err)
	}

	var evalResult struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := parseJSONResult(result, &evalResult); err != nil {
		return nil, err
	}

	if evalResult.Result.Value == "not_found" {
		return nil, fmt.Errorf("element not found: %s (ref: %s)", selector, req.Ref)
	}

	return &ActResult{OK: true, Message: fmt.Sprintf("Selected values in element: %s", req.Ref)}, nil
}

// actFillForm performs a fill form action (multiple fields at once).
func (bm *BrowserManager) actFillForm(ctx context.Context, req ActRequest) (*ActResult, error) {
	if len(req.Fields) == 0 {
		return nil, fmt.Errorf("fields are required for fill action")
	}

	filled := 0
	for _, field := range req.Fields {
		selector, err := bm.ResolveRefToSelector(field.Ref)
		if err != nil {
			continue // Skip invalid refs
		}

		switch field.Type {
		case "checkbox":
			checked := field.Value == "true" || field.Value == "1"
			js := fmt.Sprintf(`
				(function() {
					var el = document.querySelector(%q);
					if (!el) return 'not_found';
					el.checked = %v;
					el.dispatchEvent(new Event('change', { bubbles: true }));
					return 'ok';
				})()
			`, selector, checked)
			_, _ = bm.sendCDP("Runtime.evaluate", map[string]any{"expression": js})

		case "radio":
			js := fmt.Sprintf(`
				(function() {
					var el = document.querySelector(%q);
					if (!el) return 'not_found';
					el.checked = true;
					el.dispatchEvent(new Event('change', { bubbles: true }));
					return 'ok';
				})()
			`, selector)
			_, _ = bm.sendCDP("Runtime.evaluate", map[string]any{"expression": js})

		default: // textbox, combobox, etc.
			js := fmt.Sprintf(`
				(function() {
					var el = document.querySelector(%q);
					if (!el) return 'not_found';
					el.value = %q;
					el.dispatchEvent(new Event('input', { bubbles: true }));
					el.dispatchEvent(new Event('change', { bubbles: true }));
					return 'ok';
				})()
			`, selector, field.Value)
			_, _ = bm.sendCDP("Runtime.evaluate", map[string]any{"expression": js})
		}
		filled++
	}

	return &ActResult{OK: true, Message: fmt.Sprintf("Filled %d form fields", filled)}, nil
}

// actResize performs a window resize action.
func (bm *BrowserManager) actResize(ctx context.Context, req ActRequest) (*ActResult, error) {
	if req.Width <= 0 || req.Height <= 0 {
		return nil, fmt.Errorf("width and height must be positive")
	}

	// Set window bounds
	_, err := bm.sendCDP("Browser.setWindowBounds", map[string]any{
		"windowId": 1,
		"bounds": map[string]any{
			"width":  req.Width,
			"height": req.Height,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("resize failed: %w", err)
	}

	return &ActResult{OK: true, Message: fmt.Sprintf("Resized window to %dx%d", req.Width, req.Height)}, nil
}

// actWait performs a wait action.
func (bm *BrowserManager) actWait(ctx context.Context, req ActRequest) (*ActResult, error) {
	if req.TimeMs > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(req.TimeMs) * time.Millisecond):
		}
		return &ActResult{OK: true, Message: fmt.Sprintf("Waited %dms", req.TimeMs)}, nil
	}

	if req.TextGone != "" {
		// Wait for text to disappear
		deadline := time.Now().Add(time.Duration(bm.cfg.TimeoutSeconds) * time.Second)
		for time.Now().Before(deadline) {
			content, _ := bm.GetContent(ctx)
			if content == "" || !containsString(content, req.TextGone) {
				return &ActResult{OK: true, Message: fmt.Sprintf("Text '%s' is gone", req.TextGone)}, nil
			}
			time.Sleep(200 * time.Millisecond)
		}
		return nil, fmt.Errorf("timeout waiting for text to disappear: %s", req.TextGone)
	}

	return nil, fmt.Errorf("time_ms or text_gone is required for wait action")
}

// actEvaluate performs a JavaScript evaluation.
func (bm *BrowserManager) actEvaluate(ctx context.Context, req ActRequest) (*ActResult, error) {
	if req.Function == "" {
		return nil, fmt.Errorf("fn is required for evaluate action")
	}

	// If ref is provided, evaluate in element context
	if req.Ref != "" {
		selector, err := bm.ResolveRefToSelector(req.Ref)
		if err != nil {
			return nil, err
		}

		js := fmt.Sprintf(`
			(function() {
				var el = document.querySelector(%q);
				if (!el) return 'not_found';
				var fn = %s;
				return fn(el);
			})()
		`, selector, req.Function)

		result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
			"expression": js,
		})
		if err != nil {
			return nil, fmt.Errorf("evaluate failed: %w", err)
		}

		var evalResult struct {
			Result struct {
				Value any `json:"value"`
			} `json:"result"`
		}
		_ = parseJSONResult(result, &evalResult)

		return &ActResult{OK: true, Message: fmt.Sprintf("Evaluated: %v", evalResult.Result.Value)}, nil
	}

	// Evaluate in page context
	result, err := bm.sendCDP("Runtime.evaluate", map[string]any{
		"expression": req.Function,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluate failed: %w", err)
	}

	var evalResult struct {
		Result struct {
			Value any `json:"value"`
		} `json:"result"`
	}
	_ = parseJSONResult(result, &evalResult)

	return &ActResult{OK: true, Message: fmt.Sprintf("Evaluated: %v", evalResult.Result.Value)}, nil
}

// containsString checks if s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
