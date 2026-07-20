# Backend (cmd/api) image. Multi-stage build: a static Go binary on a distroless
# base. The version is baked in via ldflags AND readable at runtime from
# RP_VERSION, so the image is immutable but self-describing — exactly what Ring
# Promoter's version-aware health checks rely on.
FROM golang:1.26-alpine AS build
WORKDIR /src
# Module files first for layer caching (no deps here, but keeps the pattern).
COPY go.mod ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/api ./cmd/api

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
