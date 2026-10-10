// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
// Package sessionconfig prepares native host configuration and a local, read-only Stop reminder.
package sessionconfig

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const reminderPath = "hooks/operator-save-reminder"

// Prepare preserves image and administrator settings. Native snapshots omit symlinks,
// so projected ConfigMap files are materialized before the runner starts.
func Prepare(base, source, target, executable string, prompt bool, hostState ...string) error {
	var total int64
	copyFile := func(path, rel string) error {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("invalid host configuration file")
		}
		total += info.Size()
		if total > 4<<20 {
			return errors.New("host configuration exceeds input budget")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if rel == "settings.json" || rel == ".claude.json" {
			var incoming map[string]any
			if json.Unmarshal(b, &incoming) != nil || incoming == nil {
				return errors.New("invalid host settings")
			}
			if rel == ".claude.json" {
				// Native seeds MCP definitions, never account state or project history.
				if servers, ok := incoming["mcpServers"]; ok {
					incoming = map[string]any{"mcpServers": servers}
				} else {
					incoming = map[string]any{}
				}
			}
			if err := validateMergeSettings(incoming); err != nil {
				return err
			}
			current := map[string]any{}
			if old, err := os.ReadFile(dest); err == nil {
				if json.Unmarshal(old, &current) != nil {
					return errors.New("invalid image settings")
				}
				if err := validateMergeSettings(current); err != nil {
					return err
				}
			}
			merge(current, incoming, "")
			b, err = json.Marshal(current)
			if err != nil {
				return err
			}
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		return os.WriteFile(dest, b, 0644|(info.Mode().Perm()&0111))
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	if base != "" {
		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			rel, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			return copyFile(path, rel)
		})
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for _, path := range hostState {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := copyFile(path, ".claude.json"); err != nil {
			return err
		}
	}
	if source != "" {
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		root, err := filepath.EvalSymlinks(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "..") {
				continue
			}
			path, err := filepath.EvalSymlinks(filepath.Join(source, entry.Name()))
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return errors.New("host configuration projection escapes source")
			}
			if err := copyFile(path, entry.Name()); err != nil {
				return err
			}
		}
	}
	if !prompt {
		return nil
	}
	dest := filepath.Join(target, reminderPath)
	if _, err := os.Lstat(dest); err == nil {
		return errors.New("reserved save reminder path already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	if len(b) > 48<<20 {
		return errors.New("save reminder executable exceeds budget")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, b, 0555); err != nil {
		return err
	}
	settings := map[string]any{}
	path := filepath.Join(target, "settings.json")
	if b, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(b, &settings) != nil {
			return errors.New("invalid host settings")
		}
	}
	if disabled, _ := settings["disableAllHooks"].(bool); !disabled {
		hooks, ok := settings["hooks"].(map[string]any)
		if !ok && settings["hooks"] != nil {
			return errors.New("invalid host hooks")
		}
		if hooks == nil {
			hooks = map[string]any{}
			settings["hooks"] = hooks
		}
		stop, ok := hooks["Stop"].([]any)
		if !ok && hooks["Stop"] != nil {
			return errors.New("invalid Stop hooks")
		}
		hooks["Stop"] = append(stop, map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "\"$CLAUDE_CONFIG_DIR/hooks/operator-save-reminder\" save-reminder", "timeout": 10}}})
	}
	b, err = json.Marshal(settings)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func validateMergeSettings(settings map[string]any) error {
	if value, exists := settings["hooks"]; exists {
		hooks, ok := value.(map[string]any)
		if !ok {
			return errors.New("invalid host hooks")
		}
		for _, event := range hooks {
			if _, ok := event.([]any); !ok {
				return errors.New("invalid host hook event")
			}
		}
	}
	if value, exists := settings["permissions"]; exists {
		permissions, ok := value.(map[string]any)
		if !ok {
			return errors.New("invalid host permissions")
		}
		if value, exists := permissions["deny"]; exists {
			deny, ok := value.([]any)
			if !ok {
				return errors.New("invalid host deny rules")
			}
			for _, rule := range deny {
				if _, ok := rule.(string); !ok {
					return errors.New("invalid host deny rule")
				}
			}
		}
	}
	return nil
}

// Administrator overrides retain image hooks and permission deny rules. Other
// settings use normal recursive overlay semantics; no security settings are synthesized.
func merge(dst, src map[string]any, path string) {
	for key, value := range src {
		next := path + "/" + key
		left, lok := dst[key].(map[string]any)
		right, rok := value.(map[string]any)
		if lok && rok {
			merge(left, right, next)
			continue
		}
		if a, ok := dst[key].([]any); ok {
			if b, ok := value.([]any); ok && (path == "/hooks" || next == "/permissions/deny") {
				dst[key] = append(a, b...)
				continue
			}
		}
		dst[key] = value
	}
}
