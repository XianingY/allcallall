{{- define "allcallall.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "allcallall.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name (include "allcallall.name" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "allcallall.labels" -}}
app.kubernetes.io/name: {{ include "allcallall.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end }}

{{- define "allcallall.selectorLabels" -}}
app.kubernetes.io/name: {{ include "allcallall.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "allcallall.componentName" -}}
{{- printf "%s-%s" (include "allcallall.fullname" .root) .component | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "allcallall.serviceAccountName" -}}
{{- if .root.Values.serviceAccount.create -}}
{{- include "allcallall.componentName" . }}
{{- else -}}
{{- default "default" .root.Values.serviceAccount.name }}
{{- end -}}
{{- end }}

{{- define "allcallall.backendSecretEnv" -}}
- name: DB_DSN
  valueFrom:
    secretKeyRef:
      name: {{ .Values.external.mysql.secretName }}
      key: {{ .Values.external.mysql.dsnKey }}
- name: REDIS_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ .Values.external.redis.secretName }}
      key: {{ .Values.external.redis.passwordKey }}
      optional: true
{{- end }}

{{- define "allcallall.containerSecurityContext" -}}
allowPrivilegeEscalation: false
capabilities:
  drop: ["ALL"]
readOnlyRootFilesystem: true
runAsNonRoot: true
runAsUser: 65532
seccompProfile:
  type: RuntimeDefault
{{- end }}
