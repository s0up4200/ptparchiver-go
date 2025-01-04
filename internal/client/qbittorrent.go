// Package client provides torrent client implementations
package client

import (
	"fmt"

	qbittorrent "github.com/autobrr/go-qbittorrent"
)

// QBitClient implements TorrentClient interface for qBittorrent
type QBitClient struct {
	client *qbittorrent.Client
}

// NewQBitClient creates a new qBittorrent client
func NewQBitClient(url, username, password, basicUser, basicPass string) (*QBitClient, error) {
	qbConfig := qbittorrent.Config{
		Host:      url,
		Username:  username,
		Password:  password,
		BasicUser: basicUser,
		BasicPass: basicPass,
	}

	qb := qbittorrent.NewClient(qbConfig)
	if err := qb.Login(); err != nil {
		return nil, fmt.Errorf("failed to login to qbittorrent: %w", err)
	}

	return &QBitClient{
		client: qb,
	}, nil
}

// AddTorrent adds a torrent to qBittorrent
func (c *QBitClient) AddTorrent(torrentData []byte, name string, opts map[string]string) error {
	return c.client.AddTorrentFromMemory(torrentData, opts)
}

// GetFreeSpace returns available disk space in bytes
func (c *QBitClient) GetFreeSpace() (uint64, error) {
	return c.client.GetFreeSpaceOnDisk()
}

// CountStalledTorrents returns the number of stalled downloads in the given category
func (c *QBitClient) CountStalledTorrents(category string) (int, error) {
	torrents, err := c.client.GetTorrents(qbittorrent.TorrentFilterOptions{
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
