package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"os/exec"

	"github.com/Masterminds/semver/v3"
	"github.com/rs/zerolog/log"
)

var (
	Version string
	Commit  string
	Date    string
	BuiltBy string
)

const (
	defaultTimeout = 10 * time.Second
	apiURLFormat   = "https://api.github.com/repos/%s/%s/releases/latest"
)

// Initialize sets up version information if not already set
func Initialize() {
	if Version == "" {
		Version = getVersion()
	}
	if Commit == "" {
		Commit = getCommit()
	}
	if Date == "" {
		Date = getBuildDate()
	}
	if BuiltBy == "" {
		BuiltBy = getBuiltBy()
	}
}

func getVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	return "dev"
}

func getCommit() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				if len(setting.Value) >= 7 {
					return setting.Value[:7]
				}
				return setting.Value
			}
		}
	}
	return "none"
}

func getBuildDate() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.time" {
				return setting.Value
			}
		}
		return time.Now().UTC().Format(time.RFC3339)
	}
	return "unknown"
}

func getBuiltBy() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.username" {
				return setting.Value
			}
		}
	}

	output, err := exec.Command("git", "config", "--get", "user.name").Output()
	if err == nil && len(output) > 0 {
		return strings.TrimSpace(string(output))
	}

	return "unknown"
}

// CheckForUpdates checks GitHub for the latest release version and logs the results
func CheckForUpdates(org, repo string) error {
	if org == "" || repo == "" {
		return fmt.Errorf("organization and repository names are required")
	}

	// Show current version using structured logging
	logEvent := log.Info()

	if Version != "" {
		logEvent.Str("version", Version)
	}
	if Commit != "" && Commit != "none" {
		logEvent.Str("commit", Commit)
	}
	if Date != "" && Date != "unknown" {
		logEvent.Str("buildDate", Date)
	}
	if BuiltBy != "" && BuiltBy != "unknown" {
		logEvent.Str("builtBy", BuiltBy)
	}

	logEvent.Msg(fmt.Sprintf("%s version info", repo))

	client := &http.Client{Timeout: defaultTimeout}
	url := fmt.Sprintf(apiURLFormat, org, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", fmt.Sprintf("%s/%s", repo, Version))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("checking updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API request failed: %s", resp.Status)
	}

	var release struct {
		TagName     string    `json:"tag_name"`
		PublishedAt time.Time `json:"published_at"`
		HTMLURL     string    `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to parse GitHub response: %w", err)
	}

	// Skip version comparison for dev versions
	if Version == "dev" {
		log.Info().
			Str("current", Version).
			Str("latest", release.TagName).
			Time("publishedAt", release.PublishedAt).
			Str("updateUrl", release.HTMLURL).
			Msg("development version - skipping update check")
		return nil
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(Version, "v")

	// Parse versions using semver
	currentVer, err := semver.NewVersion(currentVersion)
	if err != nil {
		return fmt.Errorf("invalid current version format: %w", err)
	}

	latestVer, err := semver.NewVersion(latestVersion)
	if err != nil {
		return fmt.Errorf("invalid latest version format: %w", err)
	}

	if currentVer.LessThan(latestVer) {
		log.Info().
			Str("current", Version).
			Str("latest", release.TagName).
			Time("publishedAt", release.PublishedAt).
			Str("updateUrl", release.HTMLURL).
			Msg("update available")
	} else {
		log.Info().
			Time("publishedAt", release.PublishedAt).
			Msg("you are running the latest version")
	}

	return nil
}
