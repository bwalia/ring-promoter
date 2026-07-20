{{- define "rp.name" -}}ring-promoter{{- end -}}

{{- define "rp.labels" -}}
app: ring-promoter
app.kubernetes.io/name: ring-promoter
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "rp.selectorLabels" -}}
app: ring-promoter
{{- end -}}

{{/* Fail early with a clear message if the config wasn't supplied. */}}
{{- define "rp.config" -}}
{{- if not .Values.config -}}
{{- fail "values.config is empty — pass the config with `--set-file config=training/config/apps.training.yaml`" -}}
{{- end -}}
{{- .Values.config -}}
{{- end -}}
