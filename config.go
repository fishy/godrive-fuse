package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/reddit/baseplate.go/log"
	yaml "gopkg.in/yaml.v2"

	"github.com/fishy/godrive-fuse/gfs"
)

// ConfigFilename is the filename used under root config directory.
const ConfigFilename = "config.yaml"

// Default top level config values.
const (
	DefaultLogLevel = log.InfoLevel
)

// Config defines the structure of the main config file used.
type Config struct {
	// log level used, default to info level.
	LogLevel log.Level `yaml:"log_level"`

	OAuthClient OAuthClientConfig `yaml:"oauth_client"`

	HTTPClient HTTPClientConfig `yaml:"http_client"`

	Mountpoints gfs.Mountpoints `yaml:"mountpoints"`
}

// ParseConfig parses content read from config file.
func ParseConfig(f io.Reader) (cfg Config, err error) {
	err = yaml.NewDecoder(f).Decode(&cfg)
	if err != nil {
		return
	}
	// Default fallback handlings
	if cfg.LogLevel == "" {
		cfg.LogLevel = DefaultLogLevel
	}
	return
}

// ParseConfigFromDir parses the config file from root config directory.
func ParseConfigFromDir(dir string) (cfg Config, err error) {
	path := filepath.Join(dir, ConfigFilename)
	defer func() {
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(
					os.Stderr,
					"Config file %s does not exist. Please rerun %s init to create it.\n",
					path,
					os.Args[0],
				)
				os.Exit(-1)
			}
			logAndExit(err)
		}
	}()

	var f *os.File
	f, err = os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	return ParseConfig(f)
}

// DefaultConfigFile is the config file created for user to modify.
const DefaultConfigFile = `# godrive-fuse config file

# The miminal log level to keep, should be one of:
# - debug
# - info
# - warn
# - error
# - panic
# - fatal
# Default is info.
log_level:

# OAuth client related configs
oauth_client:
  # TODO
  client_id:
  client_secret:

# HTTP client related configs, controls both OAuth flow and Google Drive API
http_client:
  # A go time.Duration format string, e.g. "5s" means "5 seconds".
  # See https://pkg.go.dev/time?tab=doc#ParseDuration for more info.
  # Default is 5s
  timeout:

# A string -> string map of mountpoints.
# Keys are local directories, and values are google drive directories.
mountpoints:
  # Uncomment the next line to mount your whole google drive to /tmp/drive:
  #/tmp/drive: /
`

// In this file we cannot use baseplate log yet, so use this function to panic
func logAndExit(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	os.Exit(-1)
	// panic(args[0])
}

// InitConfigFile creates the DefaultConfigFile if it does not already exist.
func InitConfigFile(dir string) {
	path := filepath.Join(dir, ConfigFilename)
	_, err := os.Lstat(path)
	if err == nil {
		fmt.Fprintf(
			os.Stderr,
			"Config file %s already exists, leaving it alone.\n",
			path,
		)
		os.Exit(-1)
	}
	if !os.IsNotExist(err) {
		logAndExit(err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		logAndExit(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		logAndExit(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			logAndExit(err)
		}
	}()
	if _, err := io.Copy(f, strings.NewReader(DefaultConfigFile)); err != nil {
		logAndExit(err)
	}
	fmt.Printf("Config file %s created, please edit it before first use.\n", path)
}
