package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/s0up4200/ptparchiver-go/internal/archiver"
	"github.com/s0up4200/ptparchiver-go/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	version string = "dev"
	commit  string = "none"
	date    string = "unknown"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var (
	cfgFile string
	debug   bool

	rootCmd = &cobra.Command{
		Use:   "ptparchiver",
		Short: "PTP Archiver downloads and manages torrents from PTP",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
			if debug {
				zerolog.SetGlobalLevel(zerolog.DebugLevel)
			}
		},
	}

	fetchCmd = &cobra.Command{
		Use:   "fetch [container]",
		Short: "Fetch torrents for specified container or all containers",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runFetch,
		Example: `  # Fetch torrents for all containers
  ptparchiver fetch

  # Fetch torrents for a specific container
  ptparchiver fetch hetzner`,
	}

	initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize a new config file",
		RunE:  runInit,
	}

	runCmd = &cobra.Command{
		Use:   "run",
		Short: "Run the archiver service continuously",
		RunE:  runService,
		Example: `  # Run the archiver service with default interval
  ptparchiver run

  # Run with custom interval (in minutes)
  ptparchiver run --interval 30`,
	}

	interval int

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show version information and check for updates",
		RunE:  runVersion,
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")

	setupGroup := &cobra.Group{
		ID:    "setup",
		Title: "Configuration Commands:",
	}

	operationGroup := &cobra.Group{
		ID:    "operation",
		Title: "Archival Commands:",
	}

	rootCmd.AddGroup(setupGroup, operationGroup)

	initCmd.GroupID = "setup"
	runCmd.GroupID = "operation"
	fetchCmd.GroupID = "operation"

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(versionCmd)

	runCmd.Flags().IntVar(&interval, "interval", 360, "fetch interval in minutes")
}

func findConfig() (string, error) {
	if cfgFile != "" {
		return cfgFile, nil
	}

	// Check current directory
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml", nil
	}

	// Check ~/.config/ptparchiver-go/
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}

	configDir := filepath.Join(home, ".config", "ptparchiver-go")
	configPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil
	}

	return "", fmt.Errorf("no config file found in current directory or %s", configDir)
}

func loadConfig(path string) (*config.Config, error) {
	log.Debug().Str("path", path).Msg("loading config file")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

func runFetch(cmd *cobra.Command, args []string) error {
	configPath, err := findConfig()
	if err != nil {
		return err
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	client, err := archiver.NewClient(cfg, version, commit, date)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	if len(args) == 0 {
		return client.FetchAll()
	}

	return client.FetchForContainer(args[0])
}

func runInit(cmd *cobra.Command, args []string) error {
	configPath := cfgFile
	if configPath == "" {
		// Default to ~/.config/ptparchiver-go/config.yaml
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not determine home directory: %w", err)
		}
		configDir := filepath.Join(home, ".config", "ptparchiver-go")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("could not create config directory: %w", err)
		}
		configPath = filepath.Join(configDir, "config.yaml")
	}

	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file already exists at %s", configPath)
	}

	defaultConfig := config.Config{
		ApiKey:  "",
		ApiUser: "",
		BaseURL: "https://passthepopcorn.me",
		QBitClients: map[string]config.QBitConfig{
			"default": {
				URL:       "http://localhost:8080",
				Username:  "admin",
				Password:  "adminadmin",
				BasicUser: "",
				BasicPass: "",
			},
		},
		Containers: map[string]config.Container{
			"name-of-container": {
				Size:       "5T",
				MaxStalled: 5,
				Category:   "ptp-archive",
				Tags:       []string{"ptp", "archive"},
				Client:     "default",
			},
		},
		FetchSleep: 5,
		Interval:   360,
	}

	data, err := yaml.Marshal(defaultConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configContent := `# PTP Archiver Configuration
# Fill in your PTP API credentials
# Configure your qBittorrent clients
# Set up your containers with desired sizes and settings
# Read full guide at /wiki.php?action=article&id=310

`
	configContent += string(data)

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	log.Info().Str("path", configPath).Msg("created new config file")
	log.Info().Msg("remember to edit the config file and add your PTP API credentials")
	return nil
}

func runService(cmd *cobra.Command, args []string) error {
	configPath, err := findConfig()
	if err != nil {
		return err
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	if !cmd.Flags().Changed("interval") && cfg.Interval > 0 {
		interval = cfg.Interval
	}

	log.Info().
		Int("interval", interval).
		Str("schedule", fmt.Sprintf("every %d minutes", interval)).
		Msg("starting archiver service")

	client, err := archiver.NewClient(cfg, version, commit, date)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Minute)
	defer ticker.Stop()

	nextRun := time.Now().Add(time.Duration(interval) * time.Minute)

	// initial fetch
	if err := client.FetchAll(); err != nil {
		log.Error().Err(err).Msg("failed to fetch torrents")
	}
	log.Info().
		Time("nextRun", nextRun).
		Msgf("scheduling next fetch in %s", formatDuration(time.Until(nextRun)))

	for {
		select {
		case <-ticker.C:
			log.Info().Msg("performing scheduled fetch")
			if err := client.FetchAll(); err != nil {
				log.Error().Err(err).Msg("failed to fetch torrents")
			}
			nextRun = time.Now().Add(time.Duration(interval) * time.Minute)
			log.Info().
				Time("nextRun", nextRun).
				Msgf("scheduling next fetch in %s", formatDuration(time.Until(nextRun)))
		}
	}
}

// formatDuration converts a duration to a human-readable string
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60

	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%d hours %d minutes", hours, minutes)
		}
		return fmt.Sprintf("%d hours", hours)
	}
	return fmt.Sprintf("%d minutes", minutes)
}

func runVersion(cmd *cobra.Command, args []string) error {
	// Show current version using structured logging
	commitHash := "none"
	if len(commit) >= 7 {
		commitHash = commit[:7]
	}

	log.Info().
		Str("version", version).
		Str("commit", commitHash).
		Str("buildDate", date).
		Msg("PTP Archiver version info")

	// Check for latest release from GitHub API
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/s0up4200/ptparchiver-go/releases/latest", nil)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create request")
		return nil
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "PTPArchiver/"+version)

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

	if version == "dev" {
		log.Info().
			Str("latestRelease", latestVersion).
			Time("publishedAt", release.PublishedAt).
			Msg("running development version")
		return nil
	}

	if version != latestVersion {
		log.Info().
			Str("current", version).
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
