package client

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/autobrr/go-deluge"

	"github.com/s0up4200/ptparchiver-go/internal/config"
)

type DelugeClient struct {
	client interface {
		Connect(context.Context) error
		AddTorrentFile(ctx context.Context, filename, contents string, options *deluge.Options) (string, error)
		GetFreeSpace(ctx context.Context, path string) (int64, error)
		TorrentsStatus(ctx context.Context, state deluge.TorrentState, ids []string) (map[string]*deluge.TorrentStatus, error)
		LabelPlugin(ctx context.Context) (*deluge.LabelPlugin, error)
	}
}

func NewDelugeClient(cfg config.DelugeConfig) (*DelugeClient, error) {
	settings := deluge.Settings{
		Hostname:         cfg.Host,
		Port:             uint(cfg.Port),
		Login:            cfg.Username,
		Password:         cfg.Password,
		ReadWriteTimeout: time.Second * 30,
	}

	v2client := deluge.NewV2(settings)
	err := v2client.Connect(context.Background())
	if err == nil {
		return &DelugeClient{
			client: v2client,
		}, nil
	}

	v1client := deluge.NewV1(settings)
	err = v1client.Connect(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to deluge: %w", err)
	}

	return &DelugeClient{
		client: v1client,
	}, nil
}

func (c *DelugeClient) AddTorrent(torrentData []byte, name string, opts map[string]string) error {
	fileContentBase64 := base64.StdEncoding.EncodeToString(torrentData)

	options := deluge.Options{}

	if paused, ok := opts["paused"]; ok && paused == "true" {
		addPaused := true
		options.AddPaused = &addPaused
	}

	if downloadDir, ok := opts["download_dir"]; ok {
		options.DownloadLocation = &downloadDir
	}

	hash, err := c.client.AddTorrentFile(context.Background(), name, fileContentBase64, &options)
	if err != nil {
		return fmt.Errorf("failed to add torrent: %w", err)
	}

	if category, ok := opts["category"]; ok && category != "" {
		labelPlugin, err := c.client.LabelPlugin(context.Background())
		if err != nil {
			return fmt.Errorf("failed to get label plugin: %w", err)
		}

		if labelPlugin != nil {
			if err := delugeSetOrCreateTorrentLabel(context.Background(), labelPlugin, name, hash, category); err != nil {
				return fmt.Errorf("failed to set label: %w", err)
			}
		}
	}

	return nil
}

func delugeSetOrCreateTorrentLabel(ctx context.Context, plugin *deluge.LabelPlugin, clientName string, hash string, label string) error {
	err := plugin.SetTorrentLabel(ctx, hash, label)
	if err != nil {
		var rpcErr deluge.RPCError
		if errors.As(err, &rpcErr) && rpcErr.ExceptionMessage == "Unknown Label" {
			if addErr := plugin.AddLabel(ctx, label); addErr != nil {
				return fmt.Errorf("could not add label: %s on client: %s: %w", label, clientName, addErr)
			}

			if err = plugin.SetTorrentLabel(ctx, hash, label); err != nil {
				return fmt.Errorf("could not set label: %s on client: %s: %w", label, clientName, err)
			}
		} else {
			return fmt.Errorf("could not set label: %s on client: %s: %w", label, clientName, err)
		}
	}

	return nil
}

func (c *DelugeClient) GetFreeSpace() (uint64, error) {
	freeSpace, err := c.client.GetFreeSpace(context.Background(), "")
	if err != nil {
		return 0, fmt.Errorf("failed to get free space: %w", err)
	}

	return uint64(freeSpace), nil
}

func (c *DelugeClient) CountStalledTorrents(category string) (int, error) {
	torrents, err := c.client.TorrentsStatus(context.Background(), deluge.StateDownloading, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get session state: %w", err)
	}

	stalledCount := 0
	for _, torrent := range torrents {
		if torrent.State == "Downloading" && torrent.DownloadPayloadRate == 0 {
			stalledCount++
		}
	}

	return stalledCount, nil
}
