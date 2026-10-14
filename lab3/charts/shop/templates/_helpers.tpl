{{- define "shop.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "shop.selectorLabels" -}}
app.kubernetes.io/name: {{ include "shop.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "shop.podLabels" -}}
app.kubernetes.io/name: {{ include "shop.name" .root }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/part-of: {{ include "shop.name" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "shop.commonLabels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
app.kubernetes.io/name: {{ include "shop.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: {{ include "shop.name" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}
