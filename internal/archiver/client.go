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

// make sure we follow any changes made to the python version and abort if the version is outdated
// TODO: remove this if we don't need it
const apiVersion = "0.10.0"

func (e *OutdatedVersionError) Error() string {
	return fmt.Sprintf("client is out-of-date (current: %s, latest: %s). Please update to continue",
		e.CurrentVersion, e.LatestVersion)
}

type torrentInfo struct {
	Info struct {
		Name string `bencode:"name"`
	} `bencode:"info"`
}

func NewClient(cfg *config.Config, ver, commit, date string) (*Client, error) {
	logger := log.With().Str("component", "archiver").Logger()
	logger.Info().
		Str("buildVersion", ver).
		//Str("buildCommit", commit).
		//Str("buildDate", date).
		Str("apiVersion", apiVersion).
		Str("component", "archiver").
		Msg("initializing PTP archiver")

	activeClients := make(map[string]struct{})
	for _, container := range cfg.Containers {
		activeClients[container.Client] = struct{}{}
	}

	qbits := make(map[string]*qbittorrent.Client)

	// only initialize clients that are used by containers
	for name, qbitConfig := range cfg.QBitClients {
		if _, isActive := activeClients[name]; !isActive {
			logger.Debug().
				Str("client", name).
				Str("component", "archiver").
				Msg("skipping unused qBittorrent client")
			continue
		}

		logger.Debug().
			Str("client", name).
			Str("component", "archiver").
			Msg("connecting to qBittorrent client")

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
		logger.Info().
			Str("client", name).
			Str("component", "archiver").
			Msg("successfully connected to qBittorrent client")

		qbits[name] = qb
	}

	return &Client{
		cfg:   cfg,
		qbits: qbits,
		log:   logger,
	}, nil
}

// fetches a torrent file for the given container
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
	}

	if err := json.NewDecoder(resp.Body).Decode(&fetchResp); err != nil {
		return nil, fmt.Errorf("failed to decode fetch response: %w", err)
	}

	if fetchResp.Status != "Ok" {
		return nil, fmt.Errorf("PTP API returned error status")
	}

	if fetchResp.ScriptVersion != "" {
		currentVer := apiVersion
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

	c.log.Info().
		Str("status", fetchResp.Status).
		Str("containerID", fetchResp.ContainerID).
		Str("torrentID", fetchResp.TorrentID).
		Str("component", "archiver").
		Msg("received fetch response from PTP")

	return torrentData, nil
}

// countStalledTorrents returns the number of stalled downloads (not uploads) in the given category.
// This is used to enforce the maxStalled limit before fetching new torrents from PTP.
// A torrent is considered stalled when its download has stopped due to no available peers.
func (c *Client) countStalledTorrents(qb *qbittorrent.Client, category string) (int, error) {
	torrents, err := qb.GetTorrents(qbittorrent.TorrentFilterOptions{
		Category: category,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get torrents: %w", err)
	}

	stalledCount := 0
	for _, t := range torrents {
		if t.State == qbittorrent.TorrentStateStalledDl {
			stalledCount++
		}
	}

	return stalledCount, nil
}

func (c *Client) FetchForContainer(name string) error {
	container, ok := c.cfg.Containers[name]
	if !ok {
		return fmt.Errorf("container %s not found", name)
	}

	qb, ok := c.qbits[container.Client]
	if !ok {
		return fmt.Errorf("qBittorrent client %s not found", container.Client)
	}

	// check stalled downloads count in category
	stalledCount, err := c.countStalledTorrents(qb, container.Category)
	if err != nil {
		return err
	}

	c.log.Debug().
		Str("container", name).
		Str("category", container.Category).
		Int("stalledCount", stalledCount).
		Int("maxStalled", container.MaxStalled).
		Str("component", "archiver").
		Msg("checking stalled downloads")

	if stalledCount >= container.MaxStalled {
		c.log.Info().
			Str("container", name).
			Str("category", container.Category).
			Int("stalledCount", stalledCount).
			Int("maxStalled", container.MaxStalled).
			Str("component", "archiver").
			Msg("skipping fetch due to too many stalled downloads")
		return nil
	}

	c.log.Info().
		Str("container", name).
		Str("component", "archiver").
		Msg("fetching torrent for container")

	torrent, err := c.fetchFromPTP(name, container)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("container", name).
			Str("component", "archiver").
			Msg("failed to fetch torrent from PTP")
		return fmt.Errorf("failed to fetch torrent: %w", err)
	}

	// extract torrent name
	var t struct {
		Info struct {
			Name string `bencode:"name"`
		} `bencode:"info"`
	}
	if err := bencode.DecodeBytes(torrent, &t); err != nil {
		c.log.Warn().
			Err(err).
			Str("component", "archiver").
			Msg("failed to decode torrent name")
		t.Info.Name = "unknown"
	}

	opts := map[string]string{
		"category": container.Category,
	}
	if len(container.Tags) > 0 {
		opts["tags"] = joinTags(container.Tags)
	}

	err = qb.AddTorrentFromMemory(torrent, opts)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("container", name).
			Str("client", container.Client).
			Str("component", "archiver").
			Msg("failed to add torrent to qBittorrent")
		return fmt.Errorf("failed to add torrent to qbittorrent: %w", err)
	}

	c.log.Info().
		Str("container", name).
		Str("client", container.Client).
		Str("category", container.Category).
		Str("torrent", t.Info.Name).
		Str("component", "archiver").
		Msg("successfully added torrent to qBittorrent")

	return nil
}

func (c *Client) FetchAll() error {
	var errors []error
	containers := make([]string, 0, len(c.cfg.Containers))

	for name := range c.cfg.Containers {
		containers = append(containers, name)
	}

	for i, name := range containers {
		if err := c.FetchForContainer(name); err != nil {
			errors = append(errors, fmt.Errorf("container %s: %w", name, err))
		}

		// only sleep if this isn't the last container
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
