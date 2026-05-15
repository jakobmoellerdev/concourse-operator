{{- define "gvList" -}}
{{- $groupVersions := . -}}
# API Reference

!!! note "Auto-generated"
    This page is generated from Go source at `api/v1alpha1/`. Do not edit manually — run `make docs-generate` to refresh.

## API Groups
{{- range $groupVersions }}
- {{ markdownRenderGVLink . }}
{{- end }}

{{ range $groupVersions }}
{{ template "gvDetails" . }}
{{ end }}

{{- end -}}
