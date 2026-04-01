package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteConfig generates an agentgateway YAML config file from the pushed config.
// The config format matches agentgateway's NestedRawConfig structure which requires
// a top-level "config:" wrapper with "binds:" containing listeners and routes.
func WriteConfig(configDir string, cfg ConfigPayload) error {
	configPath := filepath.Join(configDir, "agentgateway.yaml")

	var b strings.Builder

	b.WriteString("config:\n")

	// Binds section (each listener becomes a bind)
	if len(cfg.Listeners) > 0 {
		b.WriteString("  binds:\n")
		for _, l := range cfg.Listeners {
			if !l.IsEnabled {
				continue
			}
			port := extractPort(l.Address)
			if port == "" {
				port = "8080"
			}
			b.WriteString(fmt.Sprintf("  - port: %s\n", port))
			b.WriteString("    listeners:\n")
			b.WriteString(fmt.Sprintf("    - name: %s\n", l.Name))
			b.WriteString("      protocol: HTTP\n")

			// Find routes for this listener
			listenerRoutes := filterRoutes(cfg.Routes, l.ID)
			if len(listenerRoutes) > 0 {
				b.WriteString("      routes:\n")
				for _, r := range listenerRoutes {
					if !r.IsEnabled {
						continue
					}

					// Find backends for this route
					routeBackends := filterBackends(cfg.Backends, r.ID)
					if len(routeBackends) > 0 {
						b.WriteString("      - backends:\n")
						b.WriteString("        - ai:\n")
						b.WriteString("            groups:\n")
						b.WriteString("            - providers:\n")
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

	if err := os.WriteFile(configPath, []byte(b.String()), 0640); err != nil {
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

	b.WriteString(fmt.Sprintf("              - name: %s\n", be.Name))
	b.WriteString("                provider:\n")
	b.WriteString(fmt.Sprintf("                  %s: {}\n", providerType))

	if be.APIKeyRef != "" {
		b.WriteString("                policies:\n")
		b.WriteString("                  backendAuth:\n")
		b.WriteString(fmt.Sprintf("                    key: %s\n", be.APIKeyRef))
	}

	if be.Model != "" {
		b.WriteString("                model:\n")
		b.WriteString(fmt.Sprintf("                  name: %s\n", be.Model))
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
