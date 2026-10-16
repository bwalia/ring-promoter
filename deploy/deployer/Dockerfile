# rp-helm-deploy runner for Ring Promoter's k8sjob deployer (see README.md).
# Pinned tools; runs as a non-root user. Build for the cluster's arch:
#   docker buildx build --platform linux/amd64 -t docker.io/bwalia/rp-deployer:<tag> --push deploy/deployer
FROM alpine:3.22

ARG TARGETARCH=amd64
# Helm 4 to match the releases already in the cluster (written by Helm 4).
ARG HELM_VERSION=v4.2.2
ARG KUBECTL_VERSION=v1.36.3

RUN apk add --no-cache ca-certificates curl git jq openssh-client \
 && cd /tmp \
 && curl -fsSLO "https://get.helm.sh/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" \
 && curl -fsSLO "https://get.helm.sh/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz.sha256sum" \
 && sha256sum -c "helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz.sha256sum" \
 && tar -xzf "helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" \
 && install -m 0755 "linux-${TARGETARCH}/helm" /usr/local/bin/helm \
 && curl -fsSLo kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" \
 && echo "$(curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl.sha256")  kubectl" | sha256sum -c - \
 && install -m 0755 kubectl /usr/local/bin/kubectl \
 && rm -rf /tmp/* \
 && adduser -D -u 10001 -h /home/deployer deployer

# GitHub's SSH host keys, from https://api.github.com/meta (verified against
# its published fingerprints). StrictHostKeyChecking stays on.
COPY github_known_hosts /etc/ssh/ssh_known_hosts
COPY rp-helm-deploy.sh /usr/local/bin/rp-helm-deploy

USER 10001
ENV HOME=/home/deployer
ENTRYPOINT ["/usr/local/bin/rp-helm-deploy"]
