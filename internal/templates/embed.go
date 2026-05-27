package templates

import "embed"

//go:embed templates/*.yaml
var TemplateFS embed.FS
