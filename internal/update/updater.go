package update

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"
)

// SelfUpdate downloads and replaces the current fcm binary.
func SelfUpdate(currentVersion, targetVersion string) error {
	if targetVersion == "" {
		latest, err := getLatestVersion("glueops/fcm")
		if err != nil {
			return fmt.Errorf("check latest version: %w", err)
		}
		if latest == currentVersion {
			fmt.Println("Already on the latest version:", currentVersion)
			return nil
		}
		targetVersion = latest
	}

	fmt.Printf("Updating fcm %s -> %s...\n", currentVersion, targetVersion)

	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "amd64"
	} else if arch == "arm64" {
		arch = "arm64"
	}

	url := fmt.Sprintf(
		"https://github.com/glueops/fcm/releases/download/%s/fcm-linux-%s",
		targetVersion, arch,
	)

	tmpPath := "/usr/local/bin/fcm.new"
	if err := downloadFile(url, tmpPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("download: %w", err)
	}

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod: %w", err)
	}

	// Atomic replace
	if err := os.Rename(tmpPath, "/usr/local/bin/fcm"); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace binary: %w", err)
	}

	fmt.Printf("Updated to fcm %s\n", targetVersion)
	return nil
}

func getLatestVersion(repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	// Simple extraction — find "tag_name" in JSON
	// Using a minimal approach to avoid importing encoding/json here
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Find tag_name in the response
	return extractTagName(string(body)), nil
}

func extractTagName(body string) string {
	key := `"tag_name":"`
	idx := indexOf(body, key)
	if idx < 0 {
		return ""
	}
	start := idx + len(key)
	end := indexOf(body[start:], `"`)
	if end < 0 {
		return ""
	}
	return body[start : start+end]
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func downloadFile(url, destPath string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, url)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
