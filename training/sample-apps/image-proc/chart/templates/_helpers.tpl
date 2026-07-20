{{/* Common template helpers shared by all Ring Promoter training charts. */}}

{{- define "app.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "app.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name (include "app.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/* Base labels shared by every workload in this release. */}}
{{- define "app.labels" -}}
app.kubernetes.io/name: {{ include "app.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: ring-promoter-training
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{/* Base selector labels (component is appended by each workload). */}}
{{- define "app.selectorLabels" -}}
app.kubernetes.io/name: {{ include "app.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* Image tag: explicit override, else chart appVersion. */}}
{{- define "app.imageTag" -}}
{{- default .Chart.AppVersion .Values.image.tag -}}
{{- end -}}

{{/* Runtime version reported by the app: explicit, else the image tag. */}}
{{- define "app.version" -}}
{{- default (include "app.imageTag" .) .Values.version -}}
{{- end -}}

{{/* Public host for the API: explicit host, else imageproc.<domain>. */}}
{{- define "app.host" -}}
{{- if .Values.ingress.host -}}
{{- .Values.ingress.host -}}
{{- else -}}
{{- printf "imageproc.%s" .Values.ingress.domain -}}
{{- end -}}
{{- end -}}

{{/* In-cluster address (host:port) of the Redis service. */}}
{{- define "app.redisAddr" -}}
{{- printf "%s-redis:%d" (include "app.fullname" .) (int .Values.redis.port) -}}
{{- end -}}
