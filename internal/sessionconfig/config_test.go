// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package sessionconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectedConfigPreservesImagePolicyAndExecutableHooks(t *testing.T) {
	base, source, target := t.TempDir(), t.TempDir(), t.TempDir()
	write := func(root, name, contents string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(base, "hooks"), 0755); err != nil {
		t.Fatal(err)
	}
	write(base, "hooks/existing", "image hook", 0555)
	write(base, "settings.json", `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"image-hook"}]}]},"permissions":{"deny":["Read(.env)"]}}`, 0444)
	if err := os.Mkdir(filepath.Join(source, "..data"), 0755); err != nil {
		t.Fatal(err)
	}
	write(source, "..data/settings.json", `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"admin-hook"}]}]},"permissions":{"deny":["Read(secret)"]}}`, 0444)
	if err := os.Symlink("..data/settings.json", filepath.Join(source, "settings.json")); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(executable, []byte("helper"), 0555); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(base, source, target, executable, true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(target, "settings.json"))
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg["hooks"].(map[string]any)["Stop"].([]any)) != 3 {
		t.Fatal("existing Stop hooks lost")
	}
	if len(cfg["permissions"].(map[string]any)["deny"].([]any)) != 2 {
		t.Fatal("deny policy lost")
	}
	for _, path := range []string{"hooks/existing", reminderPath} {
		info, err := os.Lstat(filepath.Join(target, path))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			t.Fatal("native snapshot must see executable regular files", path, err)
		}
	}
}

func TestDisabledHooksAndUnsafeProjection(t *testing.T) {
	base, target := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "settings.json"), []byte(`{"disableAllHooks":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "binary")
	_ = os.WriteFile(executable, []byte("helper"), 0555)
	if err := Prepare(base, "", target, executable, true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(target, "settings.json"))
	var cfg map[string]any
	_ = json.Unmarshal(b, &cfg)
	if cfg["disableAllHooks"] != true || cfg["hooks"] != nil {
		t.Fatal("administrator disabling hooks must be respected")
	}
	source := t.TempDir()
	_ = os.Symlink(executable, filepath.Join(source, "outside"))
	if err := Prepare("", source, t.TempDir(), executable, false); err == nil {
		t.Fatal("outside projection accepted")
	}
	if err := Prepare(base, "", t.TempDir(), "missing", false); err != nil {
		t.Fatal("disabled prompt unnecessarily requires helper", err)
	}
}

func TestOverlayCannotEraseImageDenyRulesOrHooks(t *testing.T) {
	for _, bad := range []string{`{"permissions":null}`, `{"permissions":{"deny":null}}`, `{"permissions":{"deny":"invalid"}}`, `{"hooks":null}`, `{"hooks":{"Stop":null}}`} {
		base, source := t.TempDir(), t.TempDir()
		_ = os.WriteFile(filepath.Join(base, "settings.json"), []byte(`{"permissions":{"deny":["Read(.env)"]},"hooks":{"Stop":[]}}`), 0644)
		_ = os.WriteFile(filepath.Join(source, "settings.json"), []byte(bad), 0644)
		if Prepare(base, source, t.TempDir(), "unused", false) == nil {
			t.Fatal("invalid overlay erased image policy", bad)
		}
	}
}

func TestDefaultHostMCPStateIsPreservedWithoutAccountHistory(t *testing.T) {
	base, source, target := t.TempDir(), t.TempDir(), t.TempDir()
	state := filepath.Join(t.TempDir(), ".claude.json")
	_ = os.WriteFile(state, []byte(`{"mcpServers":{"image":{"command":"image-server"}},"account":"synthetic-private-state","projects":{"private":"history"}}`), 0644)
	_ = os.WriteFile(filepath.Join(source, ".claude.json"), []byte(`{"mcpServers":{"admin":{"command":"admin-server"}}}`), 0644)
	if err := Prepare(base, source, target, "unused", false, state); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(target, ".claude.json"))
	var cfg map[string]any
	_ = json.Unmarshal(b, &cfg)
	if len(cfg) != 1 || len(cfg["mcpServers"].(map[string]any)) != 2 {
		t.Fatal("MCP definitions lost or account state copied")
	}
}
