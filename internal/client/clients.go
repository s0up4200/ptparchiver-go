package client

type TorrentClient interface {
	AddTorrent(torrentData []byte, name string, opts map[string]string) error
	GetFreeSpace() (uint64, error)
	CountStalledTorrents(category string) (int, error)
}
