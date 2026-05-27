package templates

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// VarDef defines a template variable with its metadata.
type VarDef struct {
	Description string `yaml:"description"`
	Default     string `yaml:"default"`
	Type        string `yaml:"type"`
}

// TemplatePlugin represents a single collector input plugin.
type TemplatePlugin struct {
	Type   string                 `yaml:"type"`
	Config map[string]interface{} `yaml:"config"`
}

// TemplateCollector holds the list of input plugins for a template.
type TemplateCollector struct {
	Inputs []TemplatePlugin `yaml:"inputs"`
}

// Template represents a monitoring configuration template.
type Template struct {
	Name        string                   `yaml:"name"`
	Description string                   `yaml:"description"`
	Version     string                   `yaml:"version"`
	Variables   map[string]VarDef        `yaml:"variables"`
	Collector   TemplateCollector        `yaml:"collector"`
}

// ApplyResult contains the rendered template output.
type ApplyResult struct {
	Inputs []TemplatePlugin
}

// Loader loads and manages configuration templates from the embedded filesystem.
type Loader struct {
	templates map[string]*Template
}

// NewLoader creates a new Loader by reading all YAML files from TemplateFS.
func NewLoader() (*Loader, error) {
	entries, err := TemplateFS.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("reading templates directory: %w", err)
	}

	loader := &Loader{
		templates: make(map[string]*Template),
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		data, err := TemplateFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading template %s: %w", entry.Name(), err)
		}

		var tmpl Template
		if err := yaml.Unmarshal(data, &tmpl); err != nil {
			return nil, fmt.Errorf("parsing template %s: %w", entry.Name(), err)
		}

		if tmpl.Name == "" {
			return nil, fmt.Errorf("template %s has no name", entry.Name())
		}

		loader.templates[tmpl.Name] = &tmpl
	}

	return loader, nil
}

// List returns the names of all loaded templates.
func (l *Loader) List() []string {
	names := make([]string, 0, len(l.templates))
	for name := range l.templates {
		names = append(names, name)
	}
	return names
}

// Get returns a template by name, or an error if not found.
func (l *Loader) Get(name string) (*Template, error) {
	tmpl, ok := l.templates[name]
	if !ok {
		return nil, fmt.Errorf("template %q not found", name)
	}
	return tmpl, nil
}

// Apply renders a template with the given variables, using defaults for unset variables.
func (l *Loader) Apply(tmpl *Template, vars map[string]string) (*ApplyResult, error) {
	// Build the full variable map: start with defaults, then override with provided vars.
	resolvedVars := make(map[string]string)
	for name, def := range tmpl.Variables {
		resolvedVars[name] = def.Default
	}
	for k, v := range vars {
		resolvedVars[k] = v
	}

	result := &ApplyResult{
		Inputs: make([]TemplatePlugin, 0, len(tmpl.Collector.Inputs)),
	}

	for _, input := range tmpl.Collector.Inputs {
		renderedConfig, err := renderConfig(input.Config, resolvedVars)
		if err != nil {
			return nil, fmt.Errorf("rendering input %s: %w", input.Type, err)
		}

		result.Inputs = append(result.Inputs, TemplatePlugin{
			Type:   input.Type,
			Config: renderedConfig,
		})
	}

	return result, nil
}

// renderConfig recursively applies variable substitution to a config map.
func renderConfig(config map[string]interface{}, vars map[string]string) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(config))

	for key, val := range config {
		rendered, err := renderValue(val, vars)
		if err != nil {
			return nil, fmt.Errorf("rendering key %q: %w", key, err)
		}
		result[key] = rendered
	}

	return result, nil
}

// renderValue applies template substitution to a single value.
func renderValue(val interface{}, vars map[string]string) (interface{}, error) {
	switch v := val.(type) {
	case string:
		return renderString(v, vars)
	case map[string]interface{}:
		return renderConfig(v, vars)
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			rendered, err := renderValue(item, vars)
			if err != nil {
				return nil, err
			}
			result[i] = rendered
		}
		return result, nil
	default:
		return val, nil
	}
}

// renderString applies Go text/template rendering to a string.
func renderString(s string, vars map[string]string) (string, error) {
	// If the string doesn't contain template markers, return as-is.
	if !strings.Contains(s, "{{") {
		return s, nil
	}

	tmpl, err := template.New("").Parse(s)
	if err != nil {
		return "", fmt.Errorf("parsing template string %q: %w", s, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("executing template %q: %w", s, err)
	}

	return buf.String(), nil
}
