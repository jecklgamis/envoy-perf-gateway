{{/*
Chart name and version label, per Helm's standard label conventions.
*/}}
{{- define "envoy-perf-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "envoy-perf-gateway.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "envoy-perf-gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "envoy-perf-gateway.fullname" . }}
app.kubernetes.io/component: gateway
{{- end }}

{{- define "envoy-perf-gateway.labels" -}}
helm.sh/chart: {{ include "envoy-perf-gateway.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{ include "envoy-perf-gateway.selectorLabels" . }}
{{- end }}
