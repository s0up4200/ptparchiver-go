// Package client provides interfaces and implementations for different torrent clients
package client

// TorrentClient defines the interface that all torrent clients must implement
type TorrentClient interface {
	// AddTorrent adds a new torrent to the client
	AddTorrent(torrentData []byte, name string, opts map[string]string) error

	// GetFreeSpace returns the available disk space in bytes
	GetFreeSpace() (uint64, error)

	// CountStalledTorrents returns the number of stalled downloads in the given category
	CountStalledTorrents(category string) (int, error)
}
