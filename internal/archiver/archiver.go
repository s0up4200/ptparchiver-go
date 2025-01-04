package archiver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/docker/go-units"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/s0up4200/ptparchiver-go/internal/client"
	"github.com/s0up4200/ptparchiver-go/internal/config"
	"github.com/zeebo/bencode"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
}

type Client struct {
	cfg     *config.Config
	clients map[string]client.TorrentClient
	log     zerolog.Logger
}

// make sure we're aware of any changes made to the python version
const serverVersion = "0.10.0"

type torrentInfo struct {
	Info struct {
		Name string `bencode:"name"`
	} `bencode:"info"`
}

func NewClient(cfg *config.Config, ver, commit, date string) (*Client, error) {
	logger := log.With().Logger()
	logger.Info().
		Str("buildVersion", ver).
		Str("serverVersion", serverVersion).
		Msg("initializing PTP archiver")

	// Initialize clients map
	clients := make(map[string]client.TorrentClient)

	// Find which clients are needed
	activeClients := make(map[string]struct{})
	for _, container := range cfg.Containers {
		if container.Client != "" {
			activeClients[container.Client] = struct{}{}
		}
	}

	// Initialize only the qBittorrent clients that are used
	for name, qbitConfig := range cfg.QBitClients {
		if _, isActive := activeClients[name]; !isActive {
			logger.Debug().
				Str("client", name).
				Msg("skipping unused qBittorrent client")
			continue
		}

		logger.Debug().
			Str("client", name).
			Msg("connecting to qBittorrent client")

		qb, err := client.NewQBitClient(
			qbitConfig.URL,
			qbitConfig.Username,
			qbitConfig.Password,
			qbitConfig.BasicUser,
			qbitConfig.BasicPass,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize qbittorrent client %s: %w", name, err)
		}

		logger.Info().
			Str("client", name).
			Msg("successfully connected to qBittorrent client")

		clients[name] = qb
	}

	// Initialize only the rTorrent clients that are used
	for name, rtorrConfig := range cfg.RTorrClients {
		if _, isActive := activeClients[name]; !isActive {
			logger.Debug().
				Str("client", name).
				Msg("skipping unused rTorrent client")
			continue
		}

		logger.Debug().
			Str("client", name).
			Msg("connecting to rTorrent client")

		rt, err := client.NewRTorrentClient(
			rtorrConfig.URL,
			rtorrConfig.BasicUser,
			rtorrConfig.BasicPass,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize rtorrent client %s: %w", name, err)
		}

		logger.Info().
			Str("client", name).
			Msg("successfully connected to rTorrent client")

		clients[name] = rt
	}

	return &Client{
		cfg:     cfg,
		clients: clients,
		log:     logger,
	}, nil
}

// fetches a torrent file for the given container
func (c *Client) fetchFromPTP(name string, container config.Container) ([]byte, error) {
	client := &http.Client{}

	fetchURL := fmt.Sprintf("%s/%s", c.cfg.BaseURL, "archive.php")
	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		c.log.Error().Err(err).Str("url", fetchURL).Msg("failed to create fetch request")
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
		c.log.Error().Err(err).Str("url", fetchURL).Msg("failed to fetch from PTP")
		return nil, fmt.Errorf("failed to fetch from PTP: %w", err)
	}
	defer resp.Body.Close()

	var fetchResp struct {
		Status        string      `json:"Status"`
		Error         string      `json:"Error"`
		Message       string      `json:"Message"`
		ContainerID   interface{} `json:"ContainerID"`
		ScriptVersion string      `json:"ScriptVersion"`
		TorrentID     string      `json:"TorrentID"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&fetchResp); err != nil {
		c.log.Error().Err(err).Msg("failed to decode fetch response")
		return nil, fmt.Errorf("failed to decode fetch response: %w", err)
	}

	// check version compatibility first
	if fetchResp.ScriptVersion != "" {
		// convert PTP version to semver format if needed
		serverVerStr := fetchResp.ScriptVersion
		if !strings.Contains(serverVerStr, ".") {
			serverVerStr += ".0"
		}

		serverVer, err := semver.NewVersion(serverVerStr)
		if err != nil {
			c.log.Warn().Err(err).Str("version", serverVerStr).Msg("invalid server version format")
		} else {
			currentVer, err := semver.NewVersion(serverVersion)
			if err != nil {
				c.log.Warn().Err(err).Str("version", serverVersion).Msg("invalid current version format")
			} else if serverVer.GreaterThan(currentVer) {
				c.log.Warn().
					Str("currentVersion", currentVer.String()).
					Str("pythonVersion", serverVer.String()).
					Msg("newer version of the official Python script is available - check for important changes")
			}
		}
	}

	// check for API errors
	if fetchResp.Status != "Ok" {
		errorMsg := "unknown error"
		if fetchResp.Error != "" {
			errorMsg = fetchResp.Error
		} else if fetchResp.Message != "" {
			errorMsg = fetchResp.Message
		}
		c.log.Error().Str("error", errorMsg).Msg("PTP API returned error")
		return nil, fmt.Errorf("PTP API returned error: %s", errorMsg)
	}

	downloadURL := fmt.Sprintf("%s/%s", c.cfg.BaseURL, "torrents.php")
	req, err = http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		c.log.Error().Err(err).Str("url", downloadURL).Msg("failed to create download request")
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	req.Header.Add("ApiUser", c.cfg.ApiUser)
	req.Header.Add("ApiKey", c.cfg.ApiKey)

	q = req.URL.Query()
	q.Add("action", "download")
	q.Add("id", fetchResp.TorrentID)
	req.URL.RawQuery = q.Encode()

	resp, err = client.Do(req)
	if err != nil {
		c.log.Error().Err(err).Str("url", downloadURL).Str("torrentID", fetchResp.TorrentID).Msg("failed to download torrent")
		return nil, fmt.Errorf("failed to download torrent: %w", err)
	}
	defer resp.Body.Close()

	torrentData, err := io.ReadAll(resp.Body)
	if err != nil {
		c.log.Error().Err(err).Str("torrentID", fetchResp.TorrentID).Msg("failed to read torrent data")
		return nil, fmt.Errorf("failed to read torrent data: %w", err)
	}

	c.log.Info().
		Str("status", fetchResp.Status).
		Interface("containerID", fetchResp.ContainerID).
		Str("torrentID", fetchResp.TorrentID).
		Msg("received fetch response from PTP")

	return torrentData, nil
}

func (c *Client) FetchForContainer(name string) error {
	container, ok := c.cfg.Containers[name]
	if !ok {
		c.log.Error().Str("container", name).Msg("container not found")
		return fmt.Errorf("container %s not found", name)
	}

	// Get or create appropriate client
	var torrentClient client.TorrentClient
	var err error

	if container.WatchDir != "" {
		// Use watch directory client
		torrentClient, err = client.NewWatchDirClient(container.WatchDir)
		if err != nil {
			c.log.Error().Err(err).Str("watchDir", container.WatchDir).Msg("failed to create watch directory client")
			return fmt.Errorf("failed to create watch directory client: %w", err)
		}
	} else if container.Client != "" {
		// Use qBittorrent client
		torrentClient, ok = c.clients[container.Client]
		if !ok {
			c.log.Error().Str("client", container.Client).Msg("client not found")
			return fmt.Errorf("qBittorrent client %s not found", container.Client)
		}
	} else {
		c.log.Error().Str("container", name).Msg("container must specify either watchDir or client")
		return fmt.Errorf("container %s must specify either watchDir or client", name)
	}

	// Only check stalled downloads for qBittorrent clients
	if container.Client != "" {
		// Check stalled downloads count
		stalledCount, err := torrentClient.CountStalledTorrents(container.Category)
		if err != nil {
			return err
		}

		c.log.Debug().
			Str("container", name).
			Str("category", container.Category).
			Int("stalledCount", stalledCount).
			Int("maxStalled", container.MaxStalled).
			Msg("checking stalled downloads")

		if stalledCount >= container.MaxStalled {
			c.log.Info().
				Str("container", name).
				Str("category", container.Category).
				Int("stalledCount", stalledCount).
				Int("maxStalled", container.MaxStalled).
				Msg("skipping fetch due to too many stalled downloads")
			return nil
		}
	}

	c.log.Info().
		Str("container", name).
		Msg("fetching torrent for container")

	torrent, err := c.fetchFromPTP(name, container)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("container", name).
			Msg("failed to fetch torrent from PTP")
		return fmt.Errorf("failed to fetch torrent: %w", err)
	}

	// extract torrent info
	var t struct {
		Info struct {
			Name   string `bencode:"name"`
			Length int64  `bencode:"length"`
			Files  []struct {
				Length int64    `bencode:"length"`
				Path   []string `bencode:"path"`
			} `bencode:"files"`
		} `bencode:"info"`
	}
	if err := bencode.DecodeBytes(torrent, &t); err != nil {
		c.log.Warn().
			Err(err).
			Msg("failed to decode torrent info")
		t.Info.Name = "unknown"
	}

	// Calculate total size
	var totalSize int64
	if t.Info.Length > 0 {
		totalSize = t.Info.Length
	} else {
		for _, file := range t.Info.Files {
			totalSize += file.Length
		}
	}

	// Check available disk space - skip for rTorrent clients
	if _, ok := torrentClient.(*client.RTorrentClient); ok {
		c.log.Debug().
			Str("container", name).
			Str("torrentSize", units.HumanSize(float64(totalSize))).
			Msg("skipping disk space check for rTorrent client")
	} else {
		freeSpace, err := torrentClient.GetFreeSpace()
		if err != nil {
			c.log.Warn().
				Err(err).
				Str("container", name).
				Msg("failed to get free space, skipping fetch")
			return nil
		}

		// Add some buffer (10% extra) to the required space
		requiredSpace := uint64(float64(totalSize) * 1.1)

		c.log.Debug().
			Str("container", name).
			Str("availableSpace", units.HumanSize(float64(freeSpace))).
			Str("requiredSpace", units.HumanSize(float64(requiredSpace))).
			Str("torrentSize", units.HumanSize(float64(totalSize))).
			Msg("checking disk space")

		if freeSpace < requiredSpace {
			c.log.Info().
				Str("container", name).
				Str("freeSpace", units.HumanSize(float64(freeSpace))).
				Str("requiredSpace", units.HumanSize(float64(requiredSpace))).
				Str("torrentName", t.Info.Name).
				Msg("skipping fetch due to insufficient disk space")
			return nil
		}
	}

	opts := map[string]string{
		"category": container.Category,
	}
	if len(container.Tags) > 0 {
		opts["tags"] = strings.Join(container.Tags, ",")
	}
	if container.StartPaused || container.AddPaused {
		opts["paused"] = "true"
	}

	err = torrentClient.AddTorrent(torrent, t.Info.Name, opts)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("container", name).
			Msg("failed to add torrent")
		return fmt.Errorf("failed to add torrent: %w", err)
	}

	c.log.Info().
		Str("container", name).
		Str("torrent", t.Info.Name).
		Str("size", units.HumanSize(float64(totalSize))).
		Msg("successfully added torrent")

	return nil
}

func (c *Client) FetchAll() error {
	var errors []error
	containers := make([]string, 0, len(c.cfg.Containers))

	for name := range c.cfg.Containers {
		containers = append(containers, name)
	}

	c.log.Debug().
		Int("containerCount", len(containers)).
		Msg("starting fetch for all containers")

	for i, name := range containers {
		c.log.Debug().
			Str("container", name).
			Int("index", i+1).
			Int("total", len(containers)).
			Msg("processing container")

		if err := c.FetchForContainer(name); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", name, err))
		}

		// only sleep if this isn't the last container
		if i < len(containers)-1 {
			c.log.Debug().
				Int("seconds", c.cfg.FetchSleep).
				Msg("sleeping between container fetches")
			time.Sleep(time.Duration(c.cfg.FetchSleep) * time.Second)
		}
	}

	if len(errors) > 0 {
		c.log.Error().
			Int("failedCount", len(errors)).
			Errs("errors", errors).
			Msg("failed to fetch for some containers")
		return nil
	}

	c.log.Info().Msg("successfully completed fetch for all containers")
	return nil
}
