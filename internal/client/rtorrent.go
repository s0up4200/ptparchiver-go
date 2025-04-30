package client

import (
	"context"
	"fmt"

	rtorrent "github.com/autobrr/go-rtorrent"
	"github.com/rs/zerolog/log"
)

type RTorrentClient struct {
	client *rtorrent.Client
}

func NewRTorrentClient(url, basicUser, basicPass string) (*RTorrentClient, error) {
	cfg := rtorrent.Config{
		Addr:      url,
		BasicUser: basicUser,
		BasicPass: basicPass,
	}

	rt := rtorrent.NewClient(cfg)

	if _, err := rt.Name(context.Background()); err != nil {
		log.Error().Err(err).Str("url", url).Msg("failed to connect to rtorrent")
		return nil, fmt.Errorf("failed to connect to rtorrent: %w", err)
	}

	log.Debug().Str("url", url).Msg("connected to rtorrent")
	return &RTorrentClient{
		client: rt,
	}, nil
}

func (c *RTorrentClient) AddTorrent(torrentData []byte, name string, opts map[string]string) error {
	log.Debug().
		Str("name", name).
		Interface("options", opts).
		Msg("adding torrent to rtorrent")

	var extraArgs []*rtorrent.FieldValue
	if category, ok := opts["category"]; ok {
		extraArgs = append(extraArgs, rtorrent.DLabel.SetValue(category))
	}

	if paused, ok := opts["paused"]; ok && paused == "true" {
		if err := c.client.AddTorrentStopped(context.Background(), torrentData, extraArgs...); err != nil {
			return fmt.Errorf("failed to add torrent: %w", err)
		}
	} else {
		if err := c.client.AddTorrent(context.Background(), torrentData, extraArgs...); err != nil {
			return fmt.Errorf("failed to add torrent: %w", err)
		}
	}

	return nil
}

func (c *RTorrentClient) GetFreeSpace() (uint64, error) {
	// Get free space for the default directory
	// TODO: rTorrent doesn't have a direct method for this, we'll need to implement it
	// This is a placeholder that returns 0 for now
	return 0, nil
}

func (c *RTorrentClient) CountStalledTorrents(category string) (int, error) {

	torrents, err := c.client.GetTorrents(context.Background(), rtorrent.ViewMain)
	if err != nil {
		return 0, fmt.Errorf("failed to get torrents: %w", err)
	}

	stalledCount := 0
	for _, t := range torrents {
		if t.Label != category {
			continue
		}

		status, err := c.client.GetStatus(context.Background(), t)
		if err != nil {
			continue
		}

		if !status.Completed {
			stalledCount++
		}
	}

	log.Debug().
		Str("category", category).
		Int("stalledCount", stalledCount).
		Msg("counted incomplete torrents")

	return stalledCount, nil
}
