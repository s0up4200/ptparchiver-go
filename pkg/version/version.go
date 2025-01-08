package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	runtime "runtime/debug"

	"os/exec"

	"github.com/rs/zerolog/log"
)

var (
	Version string
	Commit  string
	Date    string
	BuiltBy string
)

func init() {
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
	if info, ok := runtime.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func getCommit() string {
	if info, ok := runtime.ReadBuildInfo(); ok {
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
	if info, ok := runtime.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.time" {
				return setting.Value
			}
		}
	}
	return "unknown"
}

func getBuiltBy() string {
	if info, ok := runtime.ReadBuildInfo(); ok {
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
	// Show current version using structured logging
	log.Info().
		Str("version", Version).
		Str("commit", Commit).
		Str("buildDate", Date).
		Str("builtBy", BuiltBy).
		Msg("ptparchiver version info")

	// Check for latest release from GitHub API
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", org, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create request")
		return nil
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", fmt.Sprintf("%s/%s", repo, Version))

	resp, err := client.Do(req)
	if err != nil {
		log.Warn().Err(err).Msg("failed to check for updates")
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warn().Str("status", resp.Status).Msg("GitHub API request failed")
		return nil
	}

	var release struct {
		TagName     string    `json:"tag_name"`
		PublishedAt time.Time `json:"published_at"`
		HTMLURL     string    `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		log.Warn().Err(err).Msg("failed to parse GitHub response")
		return nil
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")

	if Version == "dev" {
		log.Info().
			Str("latestRelease", latestVersion).
			Time("publishedAt", release.PublishedAt).
			Msg("running development version")
		return nil
	}

	if Version != latestVersion {
		log.Info().
			Str("current", Version).
			Str("latest", latestVersion).
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
