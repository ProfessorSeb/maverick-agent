package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

const agentgatewayRepo = "agentgateway/agentgateway"

// ghRelease is the minimal GitHub release response we need.
type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

// ghAsset is a single release asset.
type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// EnsureAgentgateway checks whether the agentgateway binary exists at
// configDir/agentgateway. If not, it downloads the latest release from
// GitHub for the current OS/arch.
func EnsureAgentgateway(configDir string) (string, error) {
	binPath := filepath.Join(configDir, "agentgateway")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}

	if info, err := os.Stat(binPath); err == nil && !info.IsDir() {
		log.Printf("download: agentgateway already present at %s", binPath)
		return binPath, nil
	}

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	log.Printf("download: agentgateway not found, downloading for %s/%s...", goos, goarch)

	// Fetch latest release metadata
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", agentgatewayRepo)
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("download: failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: GitHub API returned %s", resp.Status)
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("download: failed to parse release: %w", err)
	}

	// Map GOOS/GOARCH to agentgateway naming conventions
	osName := goos   // linux, darwin, windows
	archName := goarch // amd64, arm64

	// Find matching asset
	assetURL := ""
	for _, a := range release.Assets {
		// Match patterns like agentgateway-linux-amd64, agentgateway-darwin-arm64, etc.
		expected := fmt.Sprintf("agentgateway-%s-%s", osName, archName)
		if a.Name == expected || a.Name == expected+".exe" {
			assetURL = a.BrowserDownloadURL
			break
		}
	}

	if assetURL == "" {
		return "", fmt.Errorf("download: no agentgateway binary found for %s/%s in release %s", goos, goarch, release.TagName)
	}

	log.Printf("download: fetching %s (%s)", release.TagName, assetURL)

	// Download the binary
	dlResp, err := http.Get(assetURL)
	if err != nil {
		return "", fmt.Errorf("download: failed to download: %w", err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: download returned %s", dlResp.Status)
	}

	// Write to a temp file then rename for atomicity
	tmpFile, err := os.CreateTemp(configDir, "agentgateway-download-*")
	if err != nil {
		return "", fmt.Errorf("download: failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := io.Copy(tmpFile, dlResp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("download: failed to write binary: %w", err)
	}
	tmpFile.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("download: failed to chmod: %w", err)
	}

	if err := os.Rename(tmpPath, binPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("download: failed to move binary: %w", err)
	}

	log.Printf("download: agentgateway installed at %s", binPath)
	return binPath, nil
}
