{{/* SPDX-License-Identifier: Apache-2.0 */}}
{{/* Vanilla k8sagentmesh helpers (P3.5 vanilla install). */}}

{{- define "k8sagentmesh.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "k8sagentmesh.fullname" -}}
{{- printf "%s" (include "k8sagentmesh.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "k8sagentmesh.labels" -}}
app.kubernetes.io/name: {{ include "k8sagentmesh.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "k8sagentmesh.selectorLabels" -}}
app.kubernetes.io/name: {{ include "k8sagentmesh.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* Image reference: empty tag falls back to the chart appVersion (repo tag). */}}
{{- define "k8sagentmesh.image" -}}
{{- $tag := .tag | default .appVersion -}}
{{ printf "%s/%s:%s" .registry .repository $tag }}
{{- end -}}

{{- define "k8sagentmesh.serviceAccountName" -}}
{{- if and .Values.rbac.serviceAccount.create (not .Values.rbac.serviceAccount.name) -}}
{{ include "k8sagentmesh.fullname" . }}
{{- else -}}
{{ default "default" .Values.rbac.serviceAccount.name }}
{{- end -}}
{{- end -}}
