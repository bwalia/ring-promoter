{{/* Common template helpers shared by all Ring Promoter training charts.
     shopping-cart is multi-service, so the label/selector helpers take a dict
     of (ctx=root context, component=backend|frontend|redis). */}}

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

{{/* Per-component resource name: <release>-<name>-<component>. */}}
{{- define "app.componentName" -}}
{{- printf "%s-%s" (include "app.fullname" .ctx) .component | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Full label set for a component. Call with (dict "ctx" $ "component" "backend"). */}}
{{- define "app.labels" -}}
app.kubernetes.io/name: {{ include "app.name" .ctx }}
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
app.kubernetes.io/managed-by: {{ .ctx.Release.Service }}
app.kubernetes.io/part-of: ring-promoter-training
app.kubernetes.io/component: {{ .component }}
helm.sh/chart: {{ printf "%s-%s" .ctx.Chart.Name .ctx.Chart.Version }}
{{- end -}}

{{/* Selector labels for a component. Call with (dict "ctx" $ "component" "backend"). */}}
{{- define "app.selectorLabels" -}}
app.kubernetes.io/name: {{ include "app.name" .ctx }}
app.kubernetes.io/instance: {{ .ctx.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{/* Runtime version reported by the Go services: explicit override, else the
     image tag, else the chart appVersion. Backend and frontend share it. */}}
{{- define "app.version" -}}
{{- default (default .Chart.AppVersion .Values.image.tag) .Values.version -}}
{{- end -}}

{{/* Image tag shared by backend and frontend: explicit override, else appVersion. */}}
{{- define "app.imageTag" -}}
{{- default .Chart.AppVersion .Values.image.tag -}}
{{- end -}}

{{/* Public host for the frontend: explicit host, else shop.<domain>. */}}
{{- define "app.frontendHost" -}}
{{- if .Values.frontend.ingress.host -}}
{{- .Values.frontend.ingress.host -}}
{{- else -}}
{{- printf "shop.%s" .Values.ingress.domain -}}
{{- end -}}
{{- end -}}

{{/* Public host for the backend API: explicit host, else <name>-api.<domain>. */}}
{{- define "app.backendHost" -}}
{{- if .Values.backend.ingress.host -}}
{{- .Values.backend.ingress.host -}}
{{- else -}}
{{- printf "%s-api.%s" (include "app.name" .) .Values.ingress.domain -}}
{{- end -}}
{{- end -}}
