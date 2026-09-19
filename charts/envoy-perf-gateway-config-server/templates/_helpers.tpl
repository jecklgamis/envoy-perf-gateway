{{/*
Chart name and version label, per Helm's standard label conventions.
*/}}
{{- define "envoy-perf-gateway-config-server.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "envoy-perf-gateway-config-server.fullname" -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "envoy-perf-gateway-config-server.selectorLabels" -}}
app.kubernetes.io/name: {{ include "envoy-perf-gateway-config-server.fullname" . }}
app.kubernetes.io/component: config-server
{{- end }}

{{- define "envoy-perf-gateway-config-server.labels" -}}
helm.sh/chart: {{ include "envoy-perf-gateway-config-server.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{ include "envoy-perf-gateway-config-server.selectorLabels" . }}
{{- end }}
