package archiver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	qbittorrent "github.com/autobrr/go-qbittorrent"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/s0up4200/ptparchiver-go/internal/config"
	"github.com/zeebo/bencode"
)

const Version = "0.10.0"

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
}

type Client struct {
	cfg   *config.Config
	qbits map[string]*qbittorrent.Client
	log   zerolog.Logger
}

type OutdatedVersionError struct {
	CurrentVersion string
	LatestVersion  string
}

// make sure we follow any changes made to the python version
func (e *OutdatedVersionError) Error() string {
	return fmt.Sprintf("client is out-of-date (current: %s, latest: %s). Please update to continue",
		e.CurrentVersion, e.LatestVersion)
}

// Add this struct for torrent metadata
type torrentInfo struct {
	Info struct {
		Name string `bencode:"name"`
	} `bencode:"info"`
}

func NewClient(cfg *config.Config) (*Client, error) {
	logger := log.With().Str("component", "archiver").Logger()
	logger.Info().Msg("initializing PTP archiver")

	qbits := make(map[string]*qbittorrent.Client)

	// Initialize qbit client for each configured client
	for name, qbitConfig := range cfg.QBitClients {
		logger.Debug().Str("client", name).Msg("connecting to qBittorrent instance")

		qbConfig := qbittorrent.Config{
			Host:      qbitConfig.URL,
			Username:  qbitConfig.Username,
			Password:  qbitConfig.Password,
			BasicUser: qbitConfig.BasicUser,
			BasicPass: qbitConfig.BasicPass,
		}

		qb := qbittorrent.NewClient(qbConfig)
		if err := qb.Login(); err != nil {
			return nil, fmt.Errorf("failed to login to qbittorrent client %s: %w", name, err)
		}
		logger.Info().Str("client", name).Msg("successfully connected to qBittorrent")

		qbits[name] = qb
	}

	return &Client{
		cfg:   cfg,
		qbits: qbits,
		log:   logger,
	}, nil
}

// fetchFromPTP fetches a torrent file from PTP for the given container
func (c *Client) fetchFromPTP(name string, container config.Container) ([]byte, error) {
	client := &http.Client{}

	fetchURL := fmt.Sprintf("%s/%s", c.cfg.BaseURL, "archive.php")
	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetch request: %w", err)
	}

	req.Header.Add("ApiUser", c.cfg.ApiUser)
	req.Header.Add("ApiKey", c.cfg.ApiKey)

	q := req.URL.Query()
	q.Add("action", "fetch")
	q.Add("ContainerName", name)
	q.Add("ContainerSize", container.Size)
	q.Add("MaxStalled", fmt.Sprintf("%d", container.MaxStalled))
	req.URL.RawQuery = q.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from PTP: %w", err)
	}
	defer resp.Body.Close()

	var fetchResp struct {
		Status        string `json:"Status"`
		ContainerID   string `json:"ContainerID"`
		ScriptVersion string `json:"ScriptVersion"`
		TorrentID     string `json:"TorrentID"`
		ArchiveID     string `json:"ArchiveID"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&fetchResp); err != nil {
		return nil, fmt.Errorf("failed to decode fetch response: %w", err)
	}

	if fetchResp.Status != "Ok" {
		return nil, fmt.Errorf("PTP API returned error status")
	}

	if fetchResp.ScriptVersion != "" {
		currentVer := Version
		serverVer := fetchResp.ScriptVersion

		if serverVer > currentVer {
			return nil, &OutdatedVersionError{
				CurrentVersion: currentVer,
				LatestVersion:  serverVer,
			}
		}
	}

	downloadURL := fmt.Sprintf("%s/%s", c.cfg.BaseURL, "torrents.php")
	req, err = http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	req.Header.Add("ApiUser", c.cfg.ApiUser)
	req.Header.Add("ApiKey", c.cfg.ApiKey)

	q = req.URL.Query()
	q.Add("action", "download")
	q.Add("id", fetchResp.TorrentID)
	q.Add("ArchiveID", fetchResp.ArchiveID)
	req.URL.RawQuery = q.Encode()

	resp, err = client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download torrent: %w", err)
	}
	defer resp.Body.Close()

	torrentData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read torrent data: %w", err)
	}

	return torrentData, nil
}

func (c *Client) FetchForContainer(name string) error {
	c.log.Info().Str("container", name).Msg("fetching torrent for container")

	container, ok := c.cfg.Containers[name]
	if !ok {
		return fmt.Errorf("container %s not found", name)
	}

	qbit, ok := c.qbits[container.Client]
	if !ok {
		return fmt.Errorf("qbittorrent client %s not found for container %s", container.Client, name)
	}

	torrent, err := c.fetchFromPTP(name, container)
	if err != nil {
		c.log.Error().Err(err).Str("container", name).Msg("failed to fetch torrent from PTP")
		return fmt.Errorf("failed to fetch torrent: %w", err)
	}

	// Extract torrent name
	var t torrentInfo
	if err := bencode.DecodeBytes(torrent, &t); err != nil {
		c.log.Warn().Err(err).Msg("failed to decode torrent name")
	}

	opts := map[string]string{
		"category": container.Category,
	}
	if len(container.Tags) > 0 {
		opts["tags"] = joinTags(container.Tags)
	}

	err = qbit.AddTorrentFromMemory(torrent, opts)
	if err != nil {
		c.log.Error().Err(err).Str("container", name).Msg("failed to add torrent to qBittorrent")
		return fmt.Errorf("failed to add torrent to qbittorrent: %w", err)
	}

	c.log.Info().
		Str("container", name).
		Str("client", container.Client).
		Str("category", container.Category).
		Str("torrent", t.Info.Name).
		Msg("successfully added torrent to qBittorrent")

	return nil
}

func (c *Client) FetchAll() error {
	var errors []error
	containers := make([]string, 0, len(c.cfg.Containers))

	// Get a sorted list of container names for consistent ordering
	for name := range c.cfg.Containers {
		containers = append(containers, name)
	}

	for i, name := range containers {
		if err := c.FetchForContainer(name); err != nil {
			errors = append(errors, fmt.Errorf("container %s: %w", name, err))
		}

		// Only sleep if this isn't the last container
		if i < len(containers)-1 {
			time.Sleep(time.Duration(c.cfg.FetchSleep) * time.Second)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to fetch for some containers: %v", errors)
	}
	return nil
}

func joinTags(tags []string) string {
	result := ""
	for i, tag := range tags {
		if i > 0 {
			result += ","
		}
		result += tag
	}
	return result
}
