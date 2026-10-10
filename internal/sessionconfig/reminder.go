// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package sessionconfig

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const reminder = "Before ending this turn, check whether any intended work remains uncommitted or unpushed. Follow the user's instructions and repository policy, and obtain any required approval before saving or pushing. Select only intended project files; exclude credentials and scratch files. If the user declined saving or pushing, respect that choice and finish. This disposable workspace is not a backup."

// Remind never writes Git state or contacts a remote. stop_hook_active prevents
// repeating the reminder when Claude returns from the same turn's Stop hook.
func Remind(input io.Reader, output io.Writer, project string) {
	b, err := io.ReadAll(io.LimitReader(input, 65537))
	if err != nil || len(b) > 65536 {
		return
	}
	var event struct {
		Active bool `json:"stop_hook_active"`
	}
	if len(strings.TrimSpace(string(b))) == 0 || !strings.HasPrefix(strings.TrimSpace(string(b)), "{") || json.Unmarshal(b, &event) != nil || event.Active || project == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	git := func(args ...string) (string, bool) {
		args = append([]string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-C", project}, args...)
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
		b, err := cmd.Output()
		return strings.TrimSpace(string(b)), err == nil
	}
	if _, ok := git("rev-parse", "--show-toplevel"); !ok {
		return
	}
	if remote, ok := git("config", "--get", "remote.origin.url"); !ok || remote == "" {
		return
	}
	status, ok := git("status", "--porcelain", "--untracked-files=normal", "--ignore-submodules=all", "--", ".", ":(exclude).claude")
	if !ok {
		return
	}
	unsaved := status != ""
	if !unsaved {
		refs, _ := git("for-each-ref", "--format=%(refname)", "refs/remotes/origin")
		args := []string{"rev-list", "--max-count=1", "HEAD", "--not", "--remotes=origin"}
		if _, ok := git("rev-parse", "--verify", "FETCH_HEAD^{commit}"); ok {
			refs += " FETCH_HEAD"
			args = append(args, "FETCH_HEAD")
		}
		if refs != "" {
			commits, ok := git(args...)
			unsaved = ok && commits != ""
		}
	}
	if unsaved {
		_ = json.NewEncoder(output).Encode(map[string]string{"decision": "block", "reason": reminder})
	}
}
