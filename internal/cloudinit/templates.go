package cloudinit

import (
	"embed"
	"fmt"
	"sort"
)

//go:embed templates/*.yaml
var templateFS embed.FS

// ListTemplates returns the names of all embedded cloud-init templates.
func ListTemplates() ([]string, error) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("read embedded templates: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// GetTemplate returns the content of an embedded cloud-init template by name.
func GetTemplate(name string) (string, error) {
	data, err := templateFS.ReadFile("templates/" + name)
	if err != nil {
		return "", fmt.Errorf("template %q not found", name)
	}
	return string(data), nil
}
