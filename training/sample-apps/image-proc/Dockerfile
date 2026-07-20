# Multi-stage, multi-target build. One build stage compiles BOTH binaries; two
# tiny distroless final stages let you pick which one to ship:
#
#   docker build --target api    -t ...rp-training-image-proc-api:vX    .
#   docker build --target worker -t ...rp-training-image-proc-worker:vX .
#
# The version is baked in via ldflags AND readable at runtime from RP_VERSION,
# so the image is immutable but self-describing — what Ring Promoter's
# version-aware health checks rely on (here via the X-App-Version header).
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
# Module files first for layer caching (no deps here, but keeps the pattern).
COPY go.mod ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/api    ./cmd/api && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/worker ./cmd/worker

FROM gcr.io/distroless/static:nonroot AS api
COPY --from=build /out/api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]

FROM gcr.io/distroless/static:nonroot AS worker
COPY --from=build /out/worker /worker
EXPOSE 9090
USER nonroot:nonroot
ENTRYPOINT ["/worker"]
