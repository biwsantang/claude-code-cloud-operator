GO ?= go
CONTROLLER_GEN := $(CURDIR)/bin/controller-gen
CONTROLLER_TOOLS_VERSION := v0.22.0
ENVTEST_VERSION ?= 1.33.0
TEST_IMAGE := golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
.PHONY: test test-api test-tools generate manifests package build verify
$(CONTROLLER_GEN):
	mkdir -p $(CURDIR)/bin
	GOBIN=$(CURDIR)/bin $(GO) install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
generate: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) object:headerFile=hack/boilerplate.go.txt,year=2026 paths="./..."
manifests: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd paths="./..." output:crd:artifacts:config=config/crd/bases
package: manifests
	python3 hack/package.py
test:
	$(GO) test -race ./...
test-tools:
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s hack -p 'test_notices.py'
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s hack -p 'test_packaging.py'
test-api:
	@if [ "$$(uname -s)" = Darwin ]; then \
	 docker run --rm --mount type=bind,src="$(CURDIR)",dst=/workspace \
	 --mount type=bind,src="$$(go env GOMODCACHE)",dst=/go/pkg/mod \
	 --mount type=volume,src=claude-operator-linux-build,dst=/root/.cache/go-build \
	 -w /workspace -e ENVTEST_VERSION=$(ENVTEST_VERSION) -e GOTOOLCHAIN=local $(TEST_IMAGE) \
	 go test -race -tags=integration ./internal/controller -v -count=1 -timeout=5m; \
	 else ENVTEST_VERSION=$(ENVTEST_VERSION) $(GO) test -race -tags=integration ./internal/controller -v -count=1 -timeout=5m; fi
build:
	$(GO) build -o bin/manager ./cmd
	$(GO) build -o bin/spawn-runner ./cmd/spawn-runner
verify: generate package test test-tools
	$(GO) vet ./...
	$(GO) mod verify
