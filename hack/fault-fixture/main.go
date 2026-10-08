// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
// Synthetic runtime for disposable kind fault tests. It never contacts Anthropic.
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "fixture-policy-check" {
		b, err := os.ReadFile(os.Args[2])
		if err != nil || !json.Valid(b) {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 4 && os.Args[1] == "fixture-intake" {
		intake(os.Args[2], os.Args[3])
		return
	}
	if len(os.Args) < 2 || os.Args[1] != "self-hosted-runner" {
		os.Exit(2)
	}
	if len(os.Args) > 2 && os.Args[2] == "orchestrator" {
		key, err := os.ReadFile("/credential/environment-secret")
		if err != nil {
			os.Exit(1)
		}
		// Capture once: a key revision must replace this process to change readiness.
		connected := string(key) == "synthetic-valid"
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"connected":%t,"fixture":true}`, connected)
		})
		server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: time.Second}
		if server.ListenAndServe() != nil {
			os.Exit(1)
		}
		return
	}
	if _, err := os.Stat("/credential/work-order.jwt"); err != nil {
		os.Exit(1)
	}
	// No network calls, credential decoding, inference or vendor registration.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop
}

func intake(order, expiry string) {
	expires, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || expires < time.Now().Add(-time.Hour).Unix() || expires > time.Now().Add(time.Hour).Unix() {
		os.Exit(2)
	}
	file, err := os.CreateTemp("/tmp", "synthetic-work-order-")
	if err != nil {
		os.Exit(1)
	}
	defer os.Remove(file.Name())
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d,"syntheticOrder":%q}`, expires, order)))
	if _, err := file.WriteString("synthetic." + payload + ".not-a-signature"); err != nil {
		os.Exit(1)
	}
	if file.Close() != nil {
		os.Exit(1)
	}
	command := exec.Command("/operator-hooks/spawn-runner")
	command.Env = append(os.Environ(), "CLAUDE_RUNNER_POOL_ID=ccpool_fault_fixture", "CLAUDE_RUNNER_ORDER_ID="+order, "CLAUDE_RUNNER_WORK_ORDER_FILE="+file.Name())
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}
