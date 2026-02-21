// Package security – safe_merge.go provides protection against prototype pollution
// when merging maps. This is a security hardening measure inspired by openclaw.
package security

import (
	"fmt"
)

// BlockedMergeKeys are keys that should never be used in map operations
// as they can lead to prototype pollution attacks.
var BlockedMergeKeys = map[string]bool{
	"__proto__":  true,
	"prototype":  true,
	"constructor": true,
}

// SafeMergeError is returned when a blocked key is detected.
type SafeMergeError struct {
	Key string
}

func (e *SafeMergeError) Error() string {
	return fmt.Sprintf("blocked key detected in merge operation: %q", e.Key)
}

// ValidateMapKeys checks if a map contains any blocked keys that could
// cause prototype pollution. Returns an error if blocked keys are found.
func ValidateMapKeys(m map[string]any) error {
	for key := range m {
		if BlockedMergeKeys[key] {
			return &SafeMergeError{Key: key}
		}
		// Recursively check nested maps
		if nested, ok := m[key].(map[string]any); ok {
			if err := ValidateMapKeys(nested); err != nil {
				return err
			}
		}
	}
	return nil
}

// SafeMerge performs a shallow merge of src into dst, but blocks
// any keys that could cause prototype pollution.
// Returns an error if blocked keys are detected in src.
func SafeMerge(dst, src map[string]any) error {
	if err := ValidateMapKeys(src); err != nil {
		return err
	}
	for key, value := range src {
		dst[key] = value
	}
	return nil
}

// SafeMergeDeep performs a deep merge of src into dst, but blocks
// any keys that could cause prototype pollution at any level.
// Returns an error if blocked keys are detected.
func SafeMergeDeep(dst, src map[string]any) error {
	if err := ValidateMapKeys(src); err != nil {
		return err
	}
	for key, value := range src {
		if nestedSrc, ok := value.(map[string]any); ok {
			if nestedDst, exists := dst[key]; exists {
				if nestedDstMap, ok := nestedDst.(map[string]any); ok {
					if err := SafeMergeDeep(nestedDstMap, nestedSrc); err != nil {
						return err
					}
					continue
				}
			}
		}
		dst[key] = value
	}
	return nil
}

// IsBlockedKey checks if a single key is in the blocked list.
func IsBlockedKey(key string) bool {
	return BlockedMergeKeys[key]
}
