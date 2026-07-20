# Multi-stage build: a static Go binary on a distroless base. The version is
# baked in via ldflags AND readable at runtime from RP_VERSION, so the image is
# immutable but self-describing — exactly what Ring Promoter's version-aware
# health checks rely on. The operator is dependency-free (stdlib only).
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
# Module files first for layer caching (no deps here, but keeps the pattern).
COPY go.mod ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/operator ./cmd

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/operator /operator
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/operator"]
