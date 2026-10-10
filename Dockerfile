# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
FROM --platform=$BUILDPLATFORM golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS build
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /workspace
ENV GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download && go mod verify
COPY api api
COPY cmd cmd
COPY internal internal
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/manager ./cmd && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/spawn-runner ./cmd/spawn-runner
RUN go version -m /out/manager > /out/manager.buildinfo.txt && \
    go version -m /out/spawn-runner > /out/spawn-runner.buildinfo.txt
# Preserve dependency/toolchain notices in each image; no offline release bundle or signing key.
FROM --platform=$BUILDPLATFORM python:3.14.8-slim-trixie@sha256:f85c5697265c178cc6887276c55fe16cf3d14ca35c3df6a5eab3b360534a55d2 AS notices
COPY --from=build /usr/local/go /usr/local/go
COPY --from=build /out/*.buildinfo.txt /out/
WORKDIR /workspace
COPY go.mod go.sum ./
COPY hack/collect-notices.py hack/collect-notices.py
ENV GOPATH=/go GOTOOLCHAIN=local
RUN --mount=type=cache,target=/go/pkg/mod \
    /usr/local/go/bin/go mod download && \
    python3 hack/collect-notices.py --directory /out --go /usr/local/go/bin/go
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3 AS hook
COPY --from=build /out/spawn-runner /spawn-runner
COPY --from=notices /out/third-party-notices.* /usr/share/licenses/claude-code-cloud-operator/
COPY LICENSE /usr/share/licenses/claude-code-cloud-operator/LICENSE
USER 1000:1000
ENTRYPOINT ["/spawn-runner"]
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3 AS manager
COPY --from=build /out/manager /manager
COPY --from=notices /out/third-party-notices.* /usr/share/licenses/claude-code-cloud-operator/
COPY LICENSE /usr/share/licenses/claude-code-cloud-operator/LICENSE
USER 65532:65532
ENTRYPOINT ["/manager"]
