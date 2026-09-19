{{/*
Chart name and version label, per Helm's standard label conventions.
*/}}
{{- define "envoy-perf-gateway.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "envoy-perf-gateway.labels" -}}
helm.sh/chart: {{ include "envoy-perf-gateway.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Gateway component name, selector labels and labels.
*/}}
{{- define "envoy-perf-gateway.gateway.fullname" -}}
{{- printf "%s-gateway" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "envoy-perf-gateway.gateway.selectorLabels" -}}
app.kubernetes.io/name: {{ include "envoy-perf-gateway.gateway.fullname" . }}
app.kubernetes.io/component: gateway
{{- end }}

{{- define "envoy-perf-gateway.gateway.labels" -}}
{{ include "envoy-perf-gateway.labels" . }}
{{ include "envoy-perf-gateway.gateway.selectorLabels" . }}
{{- end }}

{{/*
Config server component name, selector labels and labels.
*/}}
{{- define "envoy-perf-gateway.configServer.fullname" -}}
{{- printf "%s-config-server" .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "envoy-perf-gateway.configServer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "envoy-perf-gateway.configServer.fullname" . }}
app.kubernetes.io/component: config-server
{{- end }}

{{- define "envoy-perf-gateway.configServer.labels" -}}
{{ include "envoy-perf-gateway.labels" . }}
{{ include "envoy-perf-gateway.configServer.selectorLabels" . }}
{{- end }}

{{/*
In-cluster URL of the config server Service, used as the gateway's default
CONFIG_SOURCE_URL when one isn't set explicitly.
*/}}
{{- define "envoy-perf-gateway.configServer.url" -}}
{{- printf "http://%s:%d" (include "envoy-perf-gateway.configServer.fullname" .) (.Values.configServer.service.port | int) }}
{{- end }}
