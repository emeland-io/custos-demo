{{/* deploy/helm/notebook/templates/_helpers.tpl */}}
{{- define "notebook.fullname" -}}
{{- .Release.Name }}-notebook
{{- end -}}

{{- define "notebook.labels" -}}
app.kubernetes.io/name: notebook
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "notebook.selectorLabels" -}}
app.kubernetes.io/name: notebook
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
