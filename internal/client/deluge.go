package client

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/autobrr/go-deluge"
	"github.com/s0up4200/ptparchiver-go/internal/config"
)

type DelugeClient struct {
	client interface {
		Connect(context.Context) error
		AddTorrentFile(ctx context.Context, filename, contents string, options *deluge.Options) (string, error)
		GetFreeSpace(ctx context.Context, path string) (int64, error)
		SessionState(ctx context.Context) ([]string, error)
		LabelPlugin(ctx context.Context) (*deluge.LabelPlugin, error)
	}
	isV2 bool
}

// NewDelugeClient creates a new Deluge client instance
func NewDelugeClient(cfg config.DelugeConfig) (*DelugeClient, error) {
	// Try to connect using v2 first
	v2client := deluge.NewV2(deluge.Settings{
		Hostname: cfg.URL,
		Login:    cfg.Username,
		Password: cfg.Password,
	})

	err := v2client.Connect(context.Background())
	if err == nil {
		return &DelugeClient{
			client: v2client,
			isV2:   true,
		}, nil
	}

	// Fall back to v1 if v2 fails
	v1client := deluge.NewV1(deluge.Settings{
		Hostname: cfg.URL,
		Login:    cfg.Username,
		Password: cfg.Password,
	})

	err = v1client.Connect(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to deluge: %w", err)
	}

	return &DelugeClient{
		client: v1client,
		isV2:   false,
	}, nil
}

// AddTorrent implements the TorrentClient interface
func (c *DelugeClient) AddTorrent(torrentData []byte, name string, opts map[string]string) error {
	// Convert torrent data to base64
	fileContentBase64 := base64.StdEncoding.EncodeToString(torrentData)

	// Create options
	downloadDir := opts["download_dir"]
	addPaused := false
	if paused, ok := opts["paused"]; ok && paused == "true" {
		addPaused = true
	}

	options := &deluge.Options{
		DownloadLocation: &downloadDir,
		AddPaused:        &addPaused,
	}

	// Add the torrent
	hash, err := c.client.AddTorrentFile(context.Background(), name, fileContentBase64, options)
	if err != nil {
		return fmt.Errorf("failed to add torrent: %w", err)
	}

	// If a category/label is specified, set it
	if category, ok := opts["category"]; ok && category != "" {
		// Get the label plugin
		labelPlugin, err := c.client.LabelPlugin(context.Background())
		if err != nil {
			return fmt.Errorf("failed to get label plugin: %w", err)
		}

		if labelPlugin != nil {
			// First ensure the label exists
			err = labelPlugin.AddLabel(context.Background(), category)
			if err != nil {
				return fmt.Errorf("failed to create label: %w", err)
			}

			// Then set the label on the torrent
			err = labelPlugin.SetTorrentLabel(context.Background(), hash, category)
			if err != nil {
				return fmt.Errorf("failed to set torrent label: %w", err)
			}
		}
	}

	return nil
}

// GetFreeSpace implements the TorrentClient interface
func (c *DelugeClient) GetFreeSpace() (uint64, error) {
	// Get free space in the default download location
	freeSpace, err := c.client.GetFreeSpace(context.Background(), "")
	if err != nil {
		return 0, fmt.Errorf("failed to get free space: %w", err)
	}

	return uint64(freeSpace), nil
}

// CountStalledTorrents implements the TorrentClient interface
func (c *DelugeClient) CountStalledTorrents(category string) (int, error) {
	// Get the label plugin to check categories
	labelPlugin, err := c.client.LabelPlugin(context.Background())
	if err != nil {
		return 0, fmt.Errorf("failed to get label plugin: %w", err)
	}

	if labelPlugin == nil {
		return 0, fmt.Errorf("label plugin not available")
	}

	// Get all torrent IDs from the session
	torrents, err := c.client.SessionState(context.Background())
	if err != nil {
		return 0, fmt.Errorf("failed to get session state: %w", err)
	}

	// Get all torrent labels
	labelsByTorrent, err := labelPlugin.GetTorrentsLabels(deluge.StateUnspecified, torrents)
	if err != nil {
		return 0, fmt.Errorf("failed to get torrent labels: %w", err)
	}

	// Since we can't reliably get torrent status in both v1 and v2,
	// we'll consider any torrent with the matching label as potentially stalled
	stalledCount := 0
	for _, label := range labelsByTorrent {
		if label == category {
			stalledCount++
		}
	}

	return stalledCount, nil
}
