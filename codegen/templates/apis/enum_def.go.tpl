{{- define "enum_def" -}}
type {{ .Names.Camel }} string

const (
{{- range $val := .Values }}
	{{ $.Names.Camel }}_{{ $val.Clean }} {{ $.Names.Camel }} = {{ printf "%q" $val.Original }}
{{- end }}
)
{{- end -}}
