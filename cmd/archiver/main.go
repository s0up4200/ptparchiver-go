package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/s0up4200/ptparchiver-go/internal/archiver"
	"github.com/s0up4200/ptparchiver-go/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")

	rootCmd.AddCommand(fetchCmd)
	rootCmd.AddCommand(initCmd)
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

	client, err := archiver.NewClient(cfg)
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