{{/*
Expand the name of the chart.
*/}}
{{- define "ragflow.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "ragflow.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "ragflow.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "ragflow.labels" -}}
helm.sh/chart: {{ include "ragflow.chart" . }}
{{ include "ragflow.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Resolve image repository with optional global repo prefix.
If .Values.global.repo is set, replace registry part and keep image path.
Detect existing registry by first segment containing '.' or ':' or being 'localhost'.
Usage: {{ include "ragflow.imageRepo" (dict "root" . "repo" .Values.foo.image.repository) }}
*/}}
{{- define "ragflow.imageRepo" -}}
{{- $root := .root -}}
{{- $repo := .repo -}}
{{- $global := $root.Values.global -}}
{{- if and $global $global.repo }}
  {{- $parts := splitList "/" $repo -}}
  {{- $first := index $parts 0 -}}
  {{- $hasRegistry := or (regexMatch "\\." $first) (regexMatch ":" $first) (eq $first "localhost") -}}
  {{- if $hasRegistry -}}
    {{- $path := join "/" (rest $parts) -}}
    {{- printf "%s/%s" $global.repo $path -}}
  {{- else -}}
    {{- printf "%s/%s" $global.repo $repo -}}
  {{- end -}}
{{- else -}}
  {{- $repo -}}
{{- end -}}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "ragflow.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ragflow.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "ragflow.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "ragflow.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/* Shared RAGFlow configuration mounts. */}}
{{- define "ragflow.volumeMounts" -}}
- mountPath: /etc/nginx/conf.d/ragflow.conf.python
  subPath: ragflow.conf.python
  name: nginx-config-volume
- mountPath: /etc/nginx/proxy.conf
  subPath: proxy.conf
  name: nginx-config-volume
- mountPath: /etc/nginx/nginx.conf
  subPath: nginx.conf
  name: nginx-config-volume
- mountPath: /ragflow/conf/service_conf.yaml.template
  subPath: service_conf.yaml.template
  name: service-conf-volume
- mountPath: /ragflow/entrypoint.sh
  subPath: entrypoint.sh
  name: entrypoint-volume
  readOnly: true
{{- if .Values.ragflow.logs.persistence.enabled }}
- mountPath: /ragflow/logs
  name: ragflow-logs
{{- end }}
{{- with .Values.ragflow.service_conf }}
- mountPath: /ragflow/conf/local.service_conf.yaml
  subPath: local.service_conf.yaml
  name: service-conf-volume
{{- end }}
{{- with .Values.ragflow.llm_factories }}
- mountPath: /ragflow/conf/llm_factories.json
  subPath: llm_factories.json
  name: service-conf-volume
{{- end }}
{{- end }}

{{/* Shared RAGFlow configuration volumes. */}}
{{- define "ragflow.volumes" -}}
- name: nginx-config-volume
  configMap:
    name: nginx-config
- name: service-conf-volume
  configMap:
    name: ragflow-service-config
- name: entrypoint-volume
  configMap:
    name: ragflow-entrypoint
    defaultMode: 0755
{{- if .Values.ragflow.logs.persistence.enabled }}
- name: ragflow-logs
  persistentVolumeClaim:
    claimName: {{ include "ragflow.fullname" . }}-logs
{{- end }}
{{- end }}
