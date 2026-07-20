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

{{- define "app.labels" -}}
app.kubernetes.io/name: {{ include "app.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: ring-promoter-training
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

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

{{/* ServiceAccount name: explicit, else the chart fullname. */}}
{{- define "app.serviceAccountName" -}}
{{- default (include "app.fullname" .) .Values.serviceAccount.name -}}
{{- end -}}
