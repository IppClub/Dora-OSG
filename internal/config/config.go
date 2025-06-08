package config

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    Server    `yaml:"server"`
	Sync      Sync      `yaml:"sync"`
	Repos     []Repo    `yaml:"repos"`
	Storage   Storage   `yaml:"storage"`
	Download  Download  `yaml:"download"`
	RateLimit RateLimit `yaml:"rate_limit"`
	Log       Log       `yaml:"log"`
}

type Server struct {
	Port int `yaml:"port"`
}

type Sync struct {
	Interval time.Duration `yaml:"interval"`
}

type Repo struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	LFS  bool   `yaml:"lfs"`
}

type Storage struct {
	Path   string `yaml:"path"`
}

type Download struct {
	BaseURL string `yaml:"base_url"`
}

type RateLimit struct {
	RPS   int `yaml:"rps"`
	Burst int `yaml:"burst"`
}

type Log struct {
	Level      string `yaml:"level"`       // debug, info, warn, error
	Filename   string `yaml:"filename"`    // log file path
	MaxSize    int    `yaml:"max_size"`    // megabytes
	MaxBackups int    `yaml:"max_backups"` // number of backups
	MaxAge     int    `yaml:"max_age"`     // days
	Compress   bool   `yaml:"compress"`    // compress rotated files
}

var (
	config *Config
	once   sync.Once
)

// Load loads the configuration from the default config file
func Load() (*Config, error) {
	return LoadFromFile("config/config.yaml")
}

// LoadFromFile loads the configuration from the specified file
func LoadFromFile(path string) (*Config, error) {
	var err error
	once.Do(func() {
		config = &Config{}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		err = yaml.Unmarshal(data, config)
		if err != nil {
			return
		}
		// Ensure storage directories exist
		ensureDirs(config.Storage.Path)
	})
	return config, err
}

// Get returns the current configuration
func Get() *Config {
	return config
}

// ensureDirs creates necessary directories if they don't exist
func ensureDirs(basePath string) error {
	dirs := []string{
		filepath.Join(basePath, "repos"),
		filepath.Join(basePath, "zips"),
		filepath.Join(basePath, "assets"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}