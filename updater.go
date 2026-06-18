package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jbeck018/howlerops/pkg/version"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	// GitHub API endpoint for the latest release. This must point at the
	// repository that actually publishes releases (where the install.sh
	// installer and CI upload assets), not the Go module path.
	GitHubReleasesAPI = "https://api.github.com/repos/howlerops/howlerops/releases/latest"

	// GitHubReleasesPage is the human-facing releases page used as a download
	// fallback when no matching asset is found.
	GitHubReleasesPage = "https://github.com/howlerops/howlerops/releases/latest"

	// InstallScriptURL is the official installer used for in-app updates. It is
	// the same script users run via curl, so it updates the install in place.
	InstallScriptURL = "https://raw.githubusercontent.com/howlerops/howlerops/main/install.sh"

	// Update check interval (24 hours)
	UpdateCheckInterval = 24 * time.Hour
)

// currentVersion returns the running application version. It is injected at
// build time via -ldflags "-X .../pkg/version.Version=..."; unbuilt/dev runs
// report "dev" and never report an available update.
func currentVersion() string {
	return version.Version
}

// UpdateInfo represents information about an available update
type UpdateInfo struct {
	Available      bool   `json:"available"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	DownloadURL    string `json:"downloadUrl"`
	ReleaseNotes   string `json:"releaseNotes"`
	PublishedAt    string `json:"publishedAt"`
}

// GitHubRelease represents the GitHub API response for a release
type GitHubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// UpdateChecker handles checking for application updates
type UpdateChecker struct {
	app           *application.App // v3 application reference
	ctx           interface{}      // deprecated: kept for compatibility
	lastCheckTime time.Time
	latestRelease *GitHubRelease
	httpClient    *http.Client
}

// NewUpdateChecker creates a new update checker instance
func NewUpdateChecker() *UpdateChecker {
	return &UpdateChecker{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// CheckForUpdates checks if a new version is available
func (u *UpdateChecker) CheckForUpdates() (*UpdateInfo, error) {
	// Fetch latest release from GitHub
	release, err := u.fetchLatestRelease()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch latest release: %w", err)
	}

	u.latestRelease = release
	u.lastCheckTime = time.Now()

	// Parse versions
	current := currentVersion()
	currentVer := normalizeVersion(current)
	latestVer := normalizeVersion(release.TagName)

	// Never report an update for dev/unbuilt binaries — the version isn't
	// meaningful, and comparing "dev" to a real release would always nag.
	updateAvailable := currentVer != "dev" && compareVersions(latestVer, currentVer) > 0

	// Get platform-specific download URL
	downloadURL := u.getDownloadURL(release)

	return &UpdateInfo{
		Available:      updateAvailable,
		CurrentVersion: current,
		LatestVersion:  release.TagName,
		DownloadURL:    downloadURL,
		ReleaseNotes:   release.Body,
		PublishedAt:    release.PublishedAt,
	}, nil
}

// GetCurrentVersion returns the current application version
func (u *UpdateChecker) GetCurrentVersion() string {
	return currentVersion()
}

// SetApp sets the v3 application reference
func (u *UpdateChecker) SetApp(app *application.App) {
	u.app = app
}

// OpenDownloadPage opens the download page in the default browser
func (u *UpdateChecker) OpenDownloadPage() error {
	if u.latestRelease == nil {
		return fmt.Errorf("no release information available")
	}

	if u.app != nil {
		_ = u.app.Browser.OpenURL(u.latestRelease.HTMLURL)
	}
	return nil
}

// currentAppBundlePath returns the path to the running .app bundle (macOS), or
// "" if it can't be determined (e.g. a bare CLI/dev run).
func currentAppBundlePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		exe = resolved
	}
	dir := exe
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		if strings.HasSuffix(parent, ".app") {
			return parent
		}
		dir = parent
	}
}

// DownloadAndInstall updates the app in place by running the official installer
// (the same script users run via curl), targeting the current install location.
// macOS/Linux only; Windows users are directed to the release page.
func (u *UpdateChecker) DownloadAndInstall() error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("in-app update isn't supported on Windows yet — use the release page")
	}

	script := fmt.Sprintf("curl -fsSL %s | sh", InstallScriptURL)
	if appPath := currentAppBundlePath(); appPath != "" {
		// Update the app where it currently lives so we don't create a duplicate.
		installDir := filepath.Dir(appPath)
		script = fmt.Sprintf("curl -fsSL %s | sh -s -- --install-dir %q", InstallScriptURL, installDir)
	}

	cmd := exec.Command("sh", "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("update failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// RestartApp relaunches the (now updated) app and quits the current instance so
// the new version takes over. To avoid ending up with two windows open at once,
// it does NOT force a second instance: a small detached watcher waits for this
// process to fully exit and only then reopens the app, so the old instance is
// always gone before the new one starts.
func (u *UpdateChecker) RestartApp() error {
	// Windows in-app update isn't supported yet (see DownloadAndInstall), and the
	// shell-based watcher below is POSIX-only — just quit there.
	if runtime.GOOS != "windows" {
		var launch string
		if appPath := currentAppBundlePath(); appPath != "" {
			// macOS: reopen the updated .app bundle. No `open -n`, so once the old
			// instance has exited this opens exactly one fresh instance.
			launch = fmt.Sprintf("open %q", appPath)
		} else if exe, err := os.Executable(); err == nil {
			// Linux/dev: relaunch the executable directly.
			launch = fmt.Sprintf("%q", exe)
		}

		if launch != "" {
			// Wait for the current PID to disappear (bounded to ~30s in case Quit
			// hangs), then start the updated app. Started detached so it survives
			// our own Quit() — the orphaned shell is reparented and keeps running.
			pid := os.Getpid()
			script := fmt.Sprintf(
				"i=0; while kill -0 %d 2>/dev/null && [ $i -lt 150 ]; do sleep 0.2; i=$((i+1)); done; %s",
				pid, launch,
			)
			_ = exec.Command("sh", "-c", script).Start()
		}
	}

	if u.app != nil {
		u.app.Quit()
	}
	return nil
}

// fetchLatestRelease fetches the latest release from GitHub API
func (u *UpdateChecker) fetchLatestRelease() (*GitHubRelease, error) {
	req, err := http.NewRequest("GET", GitHubReleasesAPI, nil)
	if err != nil {
		return nil, err
	}

	// Add user agent to avoid rate limiting
	req.Header.Set("User-Agent", fmt.Sprintf("HowlerOps/%s", currentVersion()))

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

// getDownloadURL returns the appropriate download URL for the current platform.
//
// The desktop app is shipped as a packaged bundle archive (e.g.
// howlerops-darwin-arm64.tar.gz containing HowlerOps.app), while the release
// also carries raw CLI binaries named howlerops-cli-<os>-<arch>. We must pick
// the desktop bundle, not the CLI binary — both contain the same os/arch
// tokens, so we explicitly skip "cli" assets and prefer a .tar.gz/.zip bundle.
func (u *UpdateChecker) getDownloadURL(release *GitHubRelease) string {
	platform := runtime.GOOS
	arch := runtime.GOARCH

	matchesPlatform := func(name string) bool {
		// Skip CLI binaries and checksum sidecar files.
		if strings.Contains(name, "cli") || strings.HasSuffix(name, ".sha256") {
			return false
		}
		if !strings.Contains(name, platform) {
			return false
		}
		// macOS desktop bundles may be universal or per-arch.
		if platform == "darwin" && strings.Contains(name, "universal") {
			return true
		}
		return strings.Contains(name, arch)
	}

	// First pass: prefer a packaged bundle archive for this platform.
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if matchesPlatform(name) && (strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".zip")) {
			return asset.BrowserDownloadURL
		}
	}

	// Second pass: any matching non-CLI asset for this platform.
	for _, asset := range release.Assets {
		if matchesPlatform(strings.ToLower(asset.Name)) {
			return asset.BrowserDownloadURL
		}
	}

	// Fallback to the release page (the curl installer also lives there).
	if release.HTMLURL != "" {
		return release.HTMLURL
	}
	return GitHubReleasesPage
}

// normalizeVersion removes 'v' prefix and trims whitespace
func normalizeVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

// compareVersions compares two semantic versions
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int

		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &p1) //nolint:errcheck // p1 defaults to 0 on parse failure
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &p2) //nolint:errcheck // p2 defaults to 0 on parse failure
		}

		if p1 > p2 {
			return 1
		}
		if p1 < p2 {
			return -1
		}
	}

	return 0
}
