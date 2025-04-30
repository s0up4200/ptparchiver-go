package client

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

type WatchDirClient struct {
	watchDir string
}

func NewWatchDirClient(watchDir string) (*WatchDirClient, error) {
	if err := os.MkdirAll(watchDir, 0755); err != nil {
		log.Error().Err(err).Str("watchDir", watchDir).Msg("failed to create watch directory")
		return nil, fmt.Errorf("failed to create watch directory: %w", err)
	}

	log.Debug().Str("watchDir", watchDir).Msg("created watch directory client")
	return &WatchDirClient{
		watchDir: watchDir,
	}, nil
}

func (c *WatchDirClient) AddTorrent(torrentData []byte, name string, opts map[string]string) error {
	torrentPath := filepath.Join(c.watchDir, fmt.Sprintf("%s.torrent", name))

	if err := os.WriteFile(torrentPath, torrentData, 0644); err != nil {
		log.Error().Err(err).Str("path", torrentPath).Msg("failed to write torrent file")
		return fmt.Errorf("failed to write torrent file: %w", err)
	}

	log.Info().
		Str("path", torrentPath).
		Msg("saved torrent file to watch directory")

	return nil
}

func (c *WatchDirClient) GetFreeSpace() (uint64, error) {
	return 0, nil
}

func (c *WatchDirClient) CountStalledTorrents(category string) (int, error) {
	return 0, nil
}
