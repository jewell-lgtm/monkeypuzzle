{{/* Chart name, optionally overridden. */}}
{{- define "monkeypuzzle.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Fully-qualified app name (release-scoped). */}}
{{- define "monkeypuzzle.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "monkeypuzzle.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Common labels. */}}
{{- define "monkeypuzzle.labels" -}}
helm.sh/chart: {{ include "monkeypuzzle.chart" . }}
{{ include "monkeypuzzle.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "monkeypuzzle.selectorLabels" -}}
app.kubernetes.io/name: {{ include "monkeypuzzle.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* Name of the Secret holding credentials — existing one if given. */}}
{{- define "monkeypuzzle.secretName" -}}
{{- if .Values.existingSecret -}}
{{- .Values.existingSecret -}}
{{- else -}}
{{- include "monkeypuzzle.fullname" . -}}
{{- end -}}
{{- end -}}

{{/* Bundled Postgres service host. */}}
{{- define "monkeypuzzle.postgresHost" -}}
{{- printf "%s-postgres" (include "monkeypuzzle.fullname" .) -}}
{{- end -}}

{{/* Temporal frontend hostPort — bundled service or external. */}}
{{- define "monkeypuzzle.temporalHostPort" -}}
{{- if .Values.temporal.enabled -}}
{{- printf "%s-temporal:7233" (include "monkeypuzzle.fullname" .) -}}
{{- else -}}
{{- required "externalTemporal.hostPort is required when temporal.enabled=false" .Values.externalTemporal.hostPort -}}
{{- end -}}
{{- end -}}

{{/*
Shared environment for both server and worker. Sensitive values come from the
Secret; non-sensitive ones are inlined. DATABASE_URL lives in the Secret because
it embeds the Postgres password.
*/}}
{{- define "monkeypuzzle.env" -}}
- name: PORT
  value: "8080"
- name: PUBLIC_BASE_URL
  value: {{ required "publicBaseURL is required" .Values.publicBaseURL | quote }}
- name: SECURE_COOKIES
  value: {{ .Values.secureCookies | quote }}
- name: TEMPORAL_HOSTPORT
  value: {{ include "monkeypuzzle.temporalHostPort" . | quote }}
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ include "monkeypuzzle.secretName" . }}
      key: DATABASE_URL
- name: WORKOS_API_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "monkeypuzzle.secretName" . }}
      key: WORKOS_API_KEY
- name: WORKOS_CLIENT_ID
  valueFrom:
    secretKeyRef:
      name: {{ include "monkeypuzzle.secretName" . }}
      key: WORKOS_CLIENT_ID
- name: AUTHKIT_DOMAIN
  valueFrom:
    secretKeyRef:
      name: {{ include "monkeypuzzle.secretName" . }}
      key: AUTHKIT_DOMAIN
- name: SESSION_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ include "monkeypuzzle.secretName" . }}
      key: SESSION_SECRET
- name: TOKEN_ENCRYPTION_KEY
  valueFrom:
    secretKeyRef:
      name: {{ include "monkeypuzzle.secretName" . }}
      key: TOKEN_ENCRYPTION_KEY
{{- if .Values.workos.jwksURL }}
- name: WORKOS_JWKS_URL
  value: {{ .Values.workos.jwksURL | quote }}
{{- end }}
{{- end -}}

{{/* Resolved image reference. */}}
{{- define "monkeypuzzle.image" -}}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) -}}
{{- end -}}

{{/*
Fail loudly on misconfigurations that would silently break auth. Called from a
template that always renders (server-deployment) so it runs on every install.
*/}}
{{- define "monkeypuzzle.validate" -}}
{{- if and .Values.secureCookies (not (hasPrefix "https://" .Values.publicBaseURL)) -}}
{{- fail "secureCookies=true requires publicBaseURL to start with https:// (set secureCookies=false only for local/non-TLS testing)" -}}
{{- end -}}
{{- if and .Values.ingress.enabled (not (hasPrefix "https://" .Values.publicBaseURL)) -}}
{{- fail "ingress.enabled=true requires an https:// publicBaseURL — the OAuth redirect and MCP resource must be the public TLS URL" -}}
{{- end -}}
{{- end -}}
