package config

type Config struct {
	ApiKey        string                  `yaml:"apiKey"`
	ApiUser       string                  `yaml:"apiUser"`
	BaseURL       string                  `yaml:"baseUrl" default:"https://passthepopcorn.me"`
	QBitClients   map[string]QBitConfig   `yaml:"qbittorrent"`
	RTorrClients  map[string]RTorrConfig  `yaml:"rtorrent"`
	DelugeClients map[string]DelugeConfig `yaml:"deluge"`
	Containers    map[string]Container    `yaml:"containers"`
	FetchSleep    int                     `yaml:"fetchSleep" default:"5"`
	Interval      int                     `yaml:"interval" default:"360"`
}

type QBitConfig struct {
	URL       string `yaml:"url"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	BasicUser string `yaml:"basicUser,omitempty"`
	BasicPass string `yaml:"basicPass,omitempty"`
}

type RTorrConfig struct {
	URL       string `yaml:"url"` // SCGI or HTTP(S) URL to rTorrent's XMLRPC endpoint
	BasicUser string `yaml:"basicUser,omitempty"`
	BasicPass string `yaml:"basicPass,omitempty"`
}

type DelugeConfig struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	BasicUser string `yaml:"basicUser"`
	BasicPass string `yaml:"basicPass"`
}

type Container struct {
	// Size is the total storage allocation for this container
	// PTP will assign torrents until this total size is reached
	Size string `yaml:"size"`
	// MaxStalled sets the maximum number of partial/stalled torrents before pausing new downloads
	// Default is 0 (unlimited). Set a positive integer to limit stalled torrents
	MaxStalled int      `yaml:"maxStalled"`
	Category   string   `yaml:"category"`
	Tags       []string `yaml:"tags,omitempty"`
	Client     string   `yaml:"client,omitempty"`   // Name of the torrent client to use (optional)
	WatchDir   string   `yaml:"watchDir,omitempty"` // Directory to save .torrent files to (optional)
	// StartPaused determines if torrents should be added in a paused/stopped state
	StartPaused bool `yaml:"startPaused,omitempty"`
	// AddPaused is an alias for StartPaused for backward compatibility
	AddPaused bool `yaml:"addPaused,omitempty"`
}
