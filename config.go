package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteConfig generates an agentgateway YAML config file from the pushed config.
func WriteConfig(configDir string, cfg ConfigPayload) error {
	configPath := filepath.Join(configDir, "agentgateway.yaml")

	var b strings.Builder

	// Admin section
	b.WriteString("admin:\n")
	b.WriteString("  address: 127.0.0.1:15000\n")

	// Listeners section
	if len(cfg.Listeners) > 0 {
		b.WriteString("listeners:\n")
		for _, l := range cfg.Listeners {
			if !l.IsEnabled {
				continue
			}
			address := l.Address
			if address == "" {
				address = "0.0.0.0:8080"
			}
			b.WriteString(fmt.Sprintf("  - name: %s\n", l.Name))
			b.WriteString(fmt.Sprintf("    address: %s\n", address))

			// Find routes for this listener
			listenerRoutes := filterRoutes(cfg.Routes, l.ID)
			if len(listenerRoutes) > 0 {
				b.WriteString("    routes:\n")
				for _, r := range listenerRoutes {
					if !r.IsEnabled {
						continue
					}
					b.WriteString(fmt.Sprintf("      - name: %s\n", r.Name))
					b.WriteString("        paths:\n")
					b.WriteString(fmt.Sprintf("          - %s\n", r.PathValue))

					// Find backends for this route
					routeBackends := filterBackends(cfg.Backends, r.ID)
					if len(routeBackends) > 0 {
						b.WriteString("        backends:\n")
						for _, be := range routeBackends {
							if !be.IsEnabled {
								continue
							}
							b.WriteString(fmt.Sprintf("          - name: %s\n", be.Name))
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

// writeBackendProvider writes the provider section for a backend.
func writeBackendProvider(b *strings.Builder, be BackendConfig) {
	providerType := be.ProviderType
	if providerType == "" {
		providerType = "openai"
	}

	b.WriteString("            provider:\n")
	b.WriteString(fmt.Sprintf("              %s: {}\n", providerType))

	if be.APIKeyRef != "" {
		b.WriteString("            auth:\n")
		b.WriteString(fmt.Sprintf("              apiKey: %s\n", be.APIKeyRef))
	}

	if be.Model != "" {
		b.WriteString("            model:\n")
		b.WriteString(fmt.Sprintf("              name: %s\n", be.Model))
	}
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
