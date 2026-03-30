package templates

import (
	"fmt"
	"sort"
	"strings"
)

// Template defines a built-in VM template with preset image, cloud-init, and resource defaults.
type Template struct {
	Name        string
	Description string
	Image       string // base image name (e.g., "alpine-3.20")
	CloudInit   string // embedded cloud-init YAML content (packages/runcmd only)
	CPUs        int    // default CPUs (0 = use config default)
	Memory      int    // default memory MB (0 = use config default)
	Disk        int    // default disk GB (0 = use config default)
}

var builtinTemplates = map[string]Template{
	"alpine": {
		Name:        "alpine",
		Description: "Alpine Linux (minimal, fast boot)",
		Image:       "alpine-3.20",
	},
	"alpine-docker": {
		Name:        "alpine-docker",
		Description: "Alpine + Docker + Compose",
		Image:       "alpine-3.20",
		Memory:      1024,
		CloudInit: `packages:
  - docker
  - docker-cli-compose
runcmd:
  - rc-update add docker default
  - service docker start`,
	},
	"alpine-dev": {
		Name:        "alpine-dev",
		Description: "Alpine + Docker + dev tools",
		Image:       "alpine-3.20",
		Memory:      2048,
		CloudInit: `packages:
  - docker
  - docker-cli-compose
  - git
  - make
  - curl
  - vim
  - bash
runcmd:
  - rc-update add docker default
  - service docker start`,
	},
	"ubuntu": {
		Name:        "ubuntu",
		Description: "Ubuntu Server",
		Image:       "ubuntu-24.04",
	},
	"ubuntu-dev": {
		Name:        "ubuntu-dev",
		Description: "Ubuntu + Docker + build tools",
		Image:       "ubuntu-24.04",
		Memory:      2048,
		CloudInit: `#cloud-config
packages:
  - docker.io
  - docker-compose-v2
  - build-essential
  - git
  - curl
  - vim
  - jq
runcmd:
  - systemctl enable docker
  - systemctl start docker
`,
	},
	"debian": {
		Name:        "debian",
		Description: "Debian Server",
		Image:       "debian-12",
	},
	"rocky": {
		Name:        "rocky",
		Description: "Rocky Linux 9",
		Image:       "rocky-9",
	},
	"centos": {
		Name:        "centos",
		Description: "CentOS Stream 9",
		Image:       "centos-stream9",
	},
	"opensuse": {
		Name:        "opensuse",
		Description: "openSUSE Leap 15.6",
		Image:       "opensuse-15.6",
	},
	"k3s": {
		Name:        "k3s",
		Description: "Alpine + k3s (single-node Kubernetes)",
		Image:       "alpine-3.20",
		CPUs:        2,
		Memory:      2048,
		Disk:        20,
		CloudInit: `runcmd:
  - curl -sfL https://get.k3s.io | sh -`,
	},
	"tailscale": {
		Name:        "tailscale",
		Description: "Ubuntu + Tailscale",
		Image:       "ubuntu-24.04",
		CloudInit: `runcmd:
  - curl -fsSL https://tailscale.com/install.sh | sh
  - echo "Run: tailscale up --authkey=YOUR_KEY"`,
	},
}

// Get returns the template with the given name, or nil if not found.
func Get(name string) *Template {
	t, ok := builtinTemplates[name]
	if !ok {
		return nil
	}
	return &t
}

// List returns all built-in templates sorted by name.
func List() []Template {
	templates := make([]Template, 0, len(builtinTemplates))
	for _, t := range builtinTemplates {
		templates = append(templates, t)
	}
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Name < templates[j].Name
	})
	return templates
}

// Names returns all template names sorted, suitable for tab completion.
func Names() []string {
	names := make([]string, 0, len(builtinTemplates))
	for name := range builtinTemplates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// MergeCloudInit takes the base user-data (from defaultUserData) and merges
// the template's packages and runcmd sections into it. The template's CloudInit
// field should be a valid #cloud-config YAML with packages and/or runcmd.
func MergeCloudInit(baseUserData, templateCloudInit string) string {
	if templateCloudInit == "" {
		return baseUserData
	}

	// Parse packages and runcmd from the template cloud-init
	// Simple line-based parsing since the format is well-known
	var packages []string
	var runcmds []string

	lines := strings.Split(templateCloudInit, "\n")
	section := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "#cloud-config" || trimmed == "" {
			continue
		}
		if trimmed == "packages:" {
			section = "packages"
			continue
		}
		if trimmed == "runcmd:" {
			section = "runcmd"
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			section = ""
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimPrefix(trimmed, "- ")
			switch section {
			case "packages":
				packages = append(packages, item)
			case "runcmd":
				runcmds = append(runcmds, item)
			}
		}
	}

	// Append to the base user-data
	result := baseUserData
	if len(packages) > 0 {
		result += "packages:\n"
		for _, pkg := range packages {
			result += fmt.Sprintf("  - %s\n", pkg)
		}
	}
	if len(runcmds) > 0 {
		result += "runcmd:\n"
		for _, cmd := range runcmds {
			result += fmt.Sprintf("  - %s\n", cmd)
		}
	}

	return result
}
