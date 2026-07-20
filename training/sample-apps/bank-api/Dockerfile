# Multi-stage build: a static Go binary on a distroless nonroot base. The
# version is baked in via ldflags AND readable at runtime from RP_VERSION, so
# the image is immutable but self-describing — what Ring Promoter's version-aware
# health checks rely on. The embedded db/migrations SQL travels inside the
# binary (//go:embed), so RUN_MIGRATIONS=true needs no extra files.
FROM golang:1.26-alpine AS build
WORKDIR /src
# Module files first for layer caching.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/bank-api ./cmd

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/bank-api /bank-api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/bank-api"]
