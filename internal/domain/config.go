package domain

type Config struct {
	Version           string
	ConfigPath        string
	Host              string `toml:"host"`
	Port              int    `toml:"port"`
	LogLevel          string `toml:"logLevel"`
	LogPath           string `toml:"logPath"`
	LogMaxSize        int    `toml:"logMaxSize"`
	LogMaxBackups     int    `toml:"logMaxBackups"`
	BaseURL           string `toml:"baseUrl"`
	SessionSecret     string `toml:"sessionSecret"`
	SecureCookie      bool   `toml:"secureCookie"`
	CheckForUpdates   bool   `toml:"checkForUpdates"`
	DatabaseType      string `toml:"databaseType"`
	PostgresHost      string `toml:"postgresHost"`
	PostgresPort      int    `toml:"postgresPort"`
	PostgresDatabase  string `toml:"postgresDatabase"`
	PostgresUser      string `toml:"postgresUser"`
	PostgresPass      string `toml:"postgresPass"`
	PostgresSslMode   string `toml:"postgresSslMode"`
	SyncMaxBodySizeMB int    `toml:"syncMaxBodySizeMB"` // 0 = unlimited
	SyncHistoryLimit  int    `toml:"syncHistoryLimit"`  // 0 = disabled
}

func (c *Config) SyncMaxBodyBytes() int64 {
	if c.SyncMaxBodySizeMB <= 0 {
		return 0
	}
	return int64(c.SyncMaxBodySizeMB) << 20
}

type ConfigUpdate struct {
	Host            *string `json:"host,omitempty"`
	Port            *int    `json:"port,omitempty"`
	LogLevel        *string `json:"log_level,omitempty"`
	LogPath         *string `json:"log_path,omitempty"`
	BaseURL         *string `json:"base_url,omitempty"`
	CheckForUpdates *bool   `json:"check_for_updates,omitempty"`
}
