// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package sessionconfig

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReminderReadsLocalGitOnlyAndStopsOnce(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(string(b), err)
		}
	}
	git("init")
	git("config", "user.email", "synthetic@example.invalid")
	git("config", "user.name", "Fixture")
	git("config", "commit.gpgsign", "false")
	_ = os.WriteFile(filepath.Join(repo, "project.txt"), []byte("initial"), 0644)
	git("add", "project.txt")
	git("commit", "-m", "fixture")
	git("remote", "add", "origin", "https://example.invalid/no-network.git")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	check := func(event string, want bool) {
		t.Helper()
		var out bytes.Buffer
		Remind(strings.NewReader(event), &out, repo)
		if (out.Len() > 0) != want {
			t.Fatalf("reminder=%q want=%v", out.String(), want)
		}
	}
	check(`{}`, false)
	_ = os.Mkdir(filepath.Join(repo, ".claude"), 0755)
	_ = os.WriteFile(filepath.Join(repo, ".claude/settings.json"), []byte("{}"), 0644)
	check(`{}`, false)
	_ = os.WriteFile(filepath.Join(repo, "project.txt"), []byte("changed"), 0644)
	check(`{}`, true)
	check(`{ "stop_hook_active" : true }`, false)
	check(`bad`, false)
	git("add", "project.txt")
	git("commit", "-m", "intended work")
	check(`{}`, true)
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	check(`{}`, false)
	_ = os.WriteFile(filepath.Join(repo, "new.txt"), []byte("intended work"), 0644)
	check(`{}`, true)
	git("remote", "remove", "origin")
	check(`{}`, false)
}
