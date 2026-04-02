package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteConfig generates agentgateway config files from the pushed config.
//
// The agentgateway -f flag expects a NestedRawConfig format (top-level "config:" key).
// Listener/route/backend definitions use the LocalConfig format loaded via localXdsPath.
// So we write two files:
//   - agentgateway.yaml:  NestedRawConfig pointing to the local config
//   - local.yaml:         LocalConfig with binds/listeners/routes/backends
func WriteConfig(configDir string, cfg ConfigPayload) error {
	localPath := filepath.Join(configDir, "local.yaml")
	configPath := filepath.Join(configDir, "agentgateway.yaml")

	// Write the local config (LocalConfig format with binds)
	var b strings.Builder
	if len(cfg.Listeners) == 0 {
		// Write a valid empty config so agentgateway doesn't fail parsing null
		b.WriteString("binds: []\n")
	} else {
		b.WriteString("binds:\n")
		for _, l := range cfg.Listeners {
			if !l.IsEnabled {
				continue
			}
			port := extractPort(l.Address)
			if port == "" {
				port = "8080"
			}
			b.WriteString(fmt.Sprintf("- port: %s\n", port))
			b.WriteString("  listeners:\n")
			b.WriteString(fmt.Sprintf("  - name: %s\n", l.Name))
			b.WriteString("    protocol: HTTP\n")

			// Find routes for this listener
			listenerRoutes := filterRoutes(cfg.Routes, l.ID)
			if len(listenerRoutes) > 0 {
				b.WriteString("    routes:\n")
				for _, r := range listenerRoutes {
					if !r.IsEnabled {
						continue
					}

					// Find backends for this route
					routeBackends := filterBackends(cfg.Backends, r.ID)
					if len(routeBackends) > 0 {
						b.WriteString("    - backends:\n")
						b.WriteString("      - ai:\n")
						b.WriteString("          groups:\n")
						b.WriteString("          - providers:\n")
						for _, be := range routeBackends {
							if !be.IsEnabled {
								continue
							}
							writeBackendProvider(&b, be)
						}
					}
				}
			}
		}
	}

	if err := os.WriteFile(localPath, []byte(b.String()), 0640); err != nil {
		return fmt.Errorf("write local config file: %w", err)
	}

	// Write the main config (NestedRawConfig format) pointing to the local config
	mainCfg := fmt.Sprintf("config:\n  localXdsPath: %s\n", localPath)
	if err := os.WriteFile(configPath, []byte(mainCfg), 0640); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

// writeBackendProvider writes the provider section for a backend in the
// agentgateway config format (under groups[].providers[]).
func writeBackendProvider(b *strings.Builder, be BackendConfig) {
	providerType := be.ProviderType
	if providerType == "" {
		providerType = "openAI"
	}
	// Map common lowercase types to agentgateway's camelCase names
	switch providerType {
	case "openai":
		providerType = "openAI"
	case "anthropic":
		providerType = "anthropic"
	}

	b.WriteString(fmt.Sprintf("            - name: %s\n", be.Name))
	b.WriteString("              provider:\n")
	b.WriteString(fmt.Sprintf("                %s: {}\n", providerType))

	if be.APIKeyRef != "" {
		b.WriteString("              policies:\n")
		b.WriteString("                backendAuth:\n")
		b.WriteString(fmt.Sprintf("                  key: %s\n", be.APIKeyRef))
	}

	if be.Model != "" {
		b.WriteString("              model:\n")
		b.WriteString(fmt.Sprintf("                name: %s\n", be.Model))
	}
}

// extractPort extracts the port from an address string like "0.0.0.0:8080".
func extractPort(address string) string {
	if address == "" {
		return ""
	}
	parts := strings.Split(address, ":")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return address
}

// filterRoutes returns routes that belong to the given listener.
func filterRoutes(routes []RouteConfig, listenerID string) []RouteConfig {
	var result []RouteConfig
	for _, r := range routes {
		if r.ListenerID == listenerID {
			result = append(result, r)
		}
	}
	return result
}

// filterBackends returns backends that belong to the given route.
func filterBackends(backends []BackendConfig, routeID string) []BackendConfig {
	var result []BackendConfig
	for _, b := range backends {
		if b.RouteID == routeID {
			result = append(result, b)
		}
	}
	return result
}
