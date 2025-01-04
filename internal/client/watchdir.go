package client

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// WatchDirClient implements TorrentClient interface for watch directory based clients
type WatchDirClient struct {
	watchDir string
}

// NewWatchDirClient creates a new watch directory client
func NewWatchDirClient(watchDir string) (*WatchDirClient, error) {
	// Create watch directory if it doesn't exist
	if err := os.MkdirAll(watchDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create watch directory: %w", err)
	}

	return &WatchDirClient{
		watchDir: watchDir,
	}, nil
}

// AddTorrent saves the torrent file to the watch directory
func (c *WatchDirClient) AddTorrent(torrentData []byte, name string, opts map[string]string) error {
	torrentPath := filepath.Join(c.watchDir, fmt.Sprintf("%s.torrent", name))

	if err := os.WriteFile(torrentPath, torrentData, 0644); err != nil {
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

// CountStalledTorrents always returns 0 since watch directory can't track torrent status
func (c *WatchDirClient) CountStalledTorrents(category string) (int, error) {
	return 0, nil
}
