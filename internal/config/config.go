package config

import (
	"bytes"
	"fmt"
	"github.com/SyncYomi/SyncYomi/internal/api"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
)

var configTemplate = `# config.toml

# Hostname / IP
#
# Default: "localhost"
#
host = "{{ .host }}"

# Port
#
# Default: 8282
#
port = 8282

# Database Type
# Set database type to use. Supported: sqlite, postgres
# If not defined, sqlite will be used by default.
# Make sure postgres is installed and running before using it.
#
# Optional
#
#DatabaseType = "sqlite"

# Postgres Host
# Set postgres host to use.
#
# Optional
#
#PostgresHost = "localhost"

# Postgres Port
# Set postgres port to use.
#
# Optional
#
#PostgresPort = "5434"

# Postgres Database
# Set postgres database to use.
# If not defined, It will use the default database. (postgres)
#
# Optional
#
#PostgresDatabase = "postgres"

# Postgres User
# Set postgres user to use.
#
#PostgresUser = "SyncYomi"

# Postgres Pass
# Set postgres password to use.
#
#
#PostgresPass = "SyncYomi"

# Postgres SSL Mode
# Set which SSL mode to communicate with Postgres. 
# Options: disable, allow, prefer, require, verify-ca, verify-full
# View impact of each option in official Postgres documentation: https://www.postgresql.org/docs/current/libpq-ssl.html#LIBPQ-SSL-SSLMODE-STATEMENTS
#PostgresSslMode = "disable"

# Base url
# Set custom baseUrl eg /SyncYomi/ to serve in subdirectory.
# Not needed for subdomain, or by accessing with the :port directly.
#
# Optional
#
#baseUrl = "/SyncYomi/"

# tachiyomi-sync-server logs file
# If not defined, logs to stdout make sure it's forward slash otherwise it won't work
#
# Optional
#
#logPath = "log/SyncYomi.log"

# Log level
#
# Default: "DEBUG"
#
# Options: "ERROR", "DEBUG", "INFO", "WARN", "TRACE"
#
logLevel = "DEBUG"

# Log Max Size
#
# Default: 50
#
# Max log size in megabytes
#
#logMaxSize = 50

# Log Max Backups
#
# Default: 3
#
# Max amount of old log files
#
#logMaxBackups = 3

# Check for updates
#
checkForUpdates = true

# Session secret
#
sessionSecret = "{{ .sessionSecret }}"
`

func writeConfig(configPath string, configFile string) error {
	cfgPath := filepath.Join(configPath, configFile)

	// check if configPath exists, if not create it
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(configPath, os.ModePerm)
		if err != nil {
			log.Println(err)
			return err
		}
	}

	// check if config exists, if not create it
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		// set default host
		host := "127.0.0.1"

		if _, err := os.Stat("/.dockerenv"); err == nil {
			// docker creates a .dockerenv file at the root
			// of the directory tree inside the container.
			// if this file exists then the viewer is running
			// from inside a container so return true
			host = "0.0.0.0"
		} else if pd, _ := os.Open("/proc/1/cgroup"); pd != nil {
			defer func(pd *os.File) {
				err := pd.Close()
				if err != nil {
					log.Printf("error closing proc/cgroup: %q", err)
				}
			}(pd)
			b := make([]byte, 4096, 4096)
			_, err := pd.Read(b)
			if err != nil {
				return err
			}
			if strings.Contains(string(b), "/docker") || strings.Contains(string(b), "/lxc") {
				host = "0.0.0.0"
			}
		}

		f, err := os.Create(cfgPath)
		if err != nil { // perm 0666
			// handle failed create
			log.Printf("error creating file: %q", err)
			return err
		}
		defer func(f *os.File) {
			err := f.Close()
			if err != nil {
				log.Printf("error closing file: %q", err)
			}
		}(f)

		// generate default sessionSecret
		sessionSecret := api.GenerateSecureToken(16)

		// setup text template to inject variables into
		tmpl, err := template.New("config").Parse(configTemplate)
		if err != nil {
			return errors.Wrap(err, "could not create config template")
		}

		tmplVars := map[string]string{
			"host":          host,
			"sessionSecret": sessionSecret,
		}

		var buffer bytes.Buffer
		if err = tmpl.Execute(&buffer, &tmplVars); err != nil {
			return errors.Wrap(err, "could not write torrent url template output")
		}

		if _, err = f.WriteString(buffer.String()); err != nil {
			log.Printf("error writing contents to file: %v %q", configPath, err)
			return err
		}

		return f.Sync()
	}

	return nil
}

type Config interface {
	UpdateConfig() error
	DynamicReload(log logger.Logger)
}

type AppConfig struct {
	Config *domain.Config
	m      sync.Mutex
}

func New(configPath string, version string) *AppConfig {
	c := &AppConfig{}
	c.defaults()
	c.Config.Version = version
	c.Config.ConfigPath = configPath

	c.load(configPath)

	return c
}

func (c *AppConfig) defaults() {
	c.Config = &domain.Config{
		Version:          "dev",
		Host:             "localhost",
		Port:             8282,
		LogLevel:         "TRACE",
		LogPath:          "",
		LogMaxSize:       50,
		LogMaxBackups:    3,
		BaseURL:          "/",
		SessionSecret:    "secret-session-key",
		CheckForUpdates:  true,
		DatabaseType:     "sqlite",
		PostgresHost:     "localhost",
		PostgresPort:     5434,
		PostgresDatabase: "postgres",
		PostgresUser:     "SyncYomi",
		PostgresPass:     "SyncYomi",
		PostgresSslMode:  "disable",
	}
}

func (c *AppConfig) load(configPath string) {
	// or use viper.SetDefault(val, def)
	//viper.SetDefault("host", config.Host)
	//viper.SetDefault("port", config.Port)
	//viper.SetDefault("logLevel", config.LogLevel)
	//viper.SetDefault("logPath", config.LogPath)

	viper.SetConfigType("toml")

	// clean trailing slash from configPath
	configPath = path.Clean(configPath)

	if configPath != "" {
		//viper.SetConfigName("config")

		// check if path and file exists
		// if not, create path and file
		if err := writeConfig(configPath, "config.toml"); err != nil {
			log.Printf("write error: %q", err)
		}

		viper.SetConfigFile(path.Join(configPath, "config.toml"))
	} else {
		viper.SetConfigName("config")

		// Search config in directories
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME/.config/syncyomi")
		viper.AddConfigPath("$HOME/.syncyomi")
	}

	// read config
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("config read error: %q", err)
	}

	if err := viper.Unmarshal(&c.Config); err != nil {
		log.Fatalf("Could not unmarshal config file: %v", viper.ConfigFileUsed())
	}
}

func (c *AppConfig) DynamicReload(log logger.Logger) {
	viper.OnConfigChange(func(e fsnotify.Event) {
		c.m.Lock()

		logLevel := viper.GetString("logLevel")
		c.Config.LogLevel = logLevel
		log.SetLogLevel(c.Config.LogLevel)

		logPath := viper.GetString("logPath")
		c.Config.LogPath = logPath

		checkUpdates := viper.GetBool("checkForUpdates")
		c.Config.CheckForUpdates = checkUpdates

		log.Debug().Msg("config file reloaded!")

		c.m.Unlock()
	})
	viper.WatchConfig()

	return
}

func (c *AppConfig) UpdateConfig() error {
	file := path.Join(c.Config.ConfigPath, "config.toml")

	f, err := os.ReadFile(file)
	if err != nil {
		return errors.Wrap(err, "could not read config file: %s", file)
	}

	lines := strings.Split(string(f), "\n")
	lines = c.processLines(lines)

	output := strings.Join(lines, "\n")
	if err := os.WriteFile(file, []byte(output), 0644); err != nil {
		return errors.Wrap(err, "could not write config file: %s", file)
	}

	return nil
}

func (c *AppConfig) processLines(lines []string) []string {
	// keep track of not found values to append at bottom
	var (
		foundLineUpdate   = false
		foundLineLogLevel = false
		foundLineLogPath  = false
	)

	for i, line := range lines {
		// set checkForUpdates
		if !foundLineUpdate && strings.Contains(line, "checkForUpdates =") {
			lines[i] = fmt.Sprintf("checkForUpdates = %t", c.Config.CheckForUpdates)
			foundLineUpdate = true
		}
		if !foundLineLogLevel && strings.Contains(line, "logLevel =") {
			lines[i] = fmt.Sprintf(`logLevel = "%s"`, c.Config.LogLevel)
			foundLineLogLevel = true
		}
		if !foundLineLogPath && strings.Contains(line, "logPath =") {
			if c.Config.LogPath == "" {
				lines[i] = `#logPath = ""`
			} else {
				lines[i] = fmt.Sprintf("logPath = \"%s\"", c.Config.LogPath)
			}
			foundLineLogPath = true
		}
	}

	// append missing vars to bottom
	if !foundLineUpdate {
		lines = append(lines, "# Check for updates")
		lines = append(lines, "#")
		lines = append(lines, fmt.Sprintf("checkForUpdates = %t", c.Config.CheckForUpdates))
	}

	if !foundLineLogLevel {
		lines = append(lines, "# Log level")
		lines = append(lines, "#")
		lines = append(lines, `# Default: "DEBUG"`)
		lines = append(lines, "#")
		lines = append(lines, `# Options: "ERROR", "DEBUG", "INFO", "WARN", "TRACE"`)
		lines = append(lines, "#")
		lines = append(lines, fmt.Sprintf(`logLevel = "%s"`, c.Config.LogLevel))
	}

	if !foundLineLogPath {
		lines = append(lines, "# Log Path")
		lines = append(lines, "#")
		lines = append(lines, "# Optional")
		lines = append(lines, "#")
		if c.Config.LogPath == "" {
			lines = append(lines, `#logPath = ""`)
		} else {
			lines = append(lines, fmt.Sprintf(`logPath = "%s"`, c.Config.LogPath))
		}
	}

	return lines
}
