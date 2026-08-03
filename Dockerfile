# syntax=docker/dockerfile:1

# ---- build the embedded Next.js UI ----
# Always rebuild from web/src so internal/web/static cannot ship stale
# (a prior bug left ?group= redirecting because only Go was rebuilt).
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
ENV NEXT_TELEMETRY_DISABLED=1
RUN NEXT_OUTPUT=export npx next build \
 && mkdir -p /out \
 && cp -R out/. /out/

# ---- build the static Go binary ----
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Replace any committed embed with the fresh UI from the web stage.
RUN rm -rf internal/web/static && mkdir -p internal/web/static
COPY --from=web /out/ internal/web/static/
ARG TARGETOS=linux
ARG TARGETARCH=amd64
# Build metadata, surfaced by the app on /version and in the UI footer.
ARG VERSION=dev
ARG GIT_COMMIT=none
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}" \
      -o /out/ringpromoter ./cmd/ringpromoter

# ---- fetch kubectl for the KubectlDeployer ----
FROM build AS kubectl
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG KUBECTL_VERSION=v1.30.4
RUN wget -q -O /out/kubectl \
      "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/${TARGETOS}/${TARGETARCH}/kubectl" \
    && chmod +x /out/kubectl

# ---- minimal runtime image ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build   /out/ringpromoter /app/ringpromoter
COPY --from=kubectl /out/kubectl      /usr/local/bin/kubectl
# Baked-in local-dev config; overridden in k3s by the mounted ConfigMap
# (RP_CONFIG_FILE=/etc/ringpromoter/config.yaml).
COPY config.yaml /app/config.yaml
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/ringpromoter"]
