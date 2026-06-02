{{- define "evms.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "evms.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "evms.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "evms.nats.url" -}}
{{- if .Values.nats.auth.enabled -}}
{{- printf "nats://%s:%s@nats:%d" .user .password 4222 -}}
{{- else -}}
{{- printf "nats://nats:%d" 4222 -}}
{{- end -}}
{{- end -}}

{{- define "evms.labels" -}}
helm.sh/chart: {{ include "evms.chart" . }}
{{ include "evms.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "evms.selectorLabels" -}}
app.kubernetes.io/name: {{ include "evms.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "evms.image" -}}
{{- $registry := .Values.global.imageRegistry -}}
{{- $tag := .Values.global.imageTag -}}
{{- $image := .image -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $image $tag -}}
{{- else -}}
{{- printf "%s:%s" $image $tag -}}
{{- end -}}
{{- end -}}
