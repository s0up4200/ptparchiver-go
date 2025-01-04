// Package client provides torrent client implementations
package client

import (
	"fmt"

	qbittorrent "github.com/autobrr/go-qbittorrent"
	"github.com/rs/zerolog/log"
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
		log.Error().Err(err).Str("url", url).Msg("failed to login to qbittorrent")
		return nil, fmt.Errorf("failed to login to qbittorrent: %w", err)
	}

	log.Debug().Str("url", url).Msg("connected to qbittorrent")
	return &QBitClient{
		client: qb,
	}, nil
}

// AddTorrent adds a torrent to qBittorrent
func (c *QBitClient) AddTorrent(torrentData []byte, name string, opts map[string]string) error {
	log.Debug().
		Str("name", name).
		Interface("options", opts).
		Msg("adding torrent to qbittorrent")
	return c.client.AddTorrentFromMemory(torrentData, opts)
}

// GetFreeSpace returns available disk space in bytes
func (c *QBitClient) GetFreeSpace() (uint64, error) {
	space, err := c.client.GetFreeSpaceOnDisk()
	if err != nil {
		log.Error().Err(err).Msg("failed to get free space")
	}
	return space, err
}

// CountStalledTorrents returns the number of stalled downloads in the given category
func (c *QBitClient) CountStalledTorrents(category string) (int, error) {
	torrents, err := c.client.GetTorrents(qbittorrent.TorrentFilterOptions{
		Category: category,
	})
	if err != nil {
		log.Error().Err(err).Str("category", category).Msg("failed to get torrents")
		return 0, fmt.Errorf("failed to get torrents: %w", err)
	}

	stalledCount := 0
	for _, t := range torrents {
		if t.State == qbittorrent.TorrentStateStalledDl {
			stalledCount++
		}
	}

	log.Debug().
		Str("category", category).
		Int("stalledCount", stalledCount).
		Msg("counted stalled torrents")

	return stalledCount, nil
}
