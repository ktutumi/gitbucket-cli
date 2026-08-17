package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	DefaultURL string                `json:"default_url"`
	Hosts      map[string]HostConfig `json:"hosts,omitempty"`
	Aliases    map[string]string     `json:"aliases,omitempty"`
}

type HostConfig struct {
	Token  string `json:"token,omitempty"`
	Legacy bool   `json:"legacy,omitempty"`
	User   string `json:"user,omitempty"`
}

type Resolved struct {
	URL    string
	Token  string
	Legacy bool
	User   string
}

type Store struct {
	path string
}

func NewStore(path string) Store {
	if path == "" {
		path = DefaultPath()
	}
	return Store{path: path}
}

func DefaultPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "gitbucket-cli", "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "gitbucket-cli", "config.json")
	}
	return filepath.Join(home, ".config", "gitbucket-cli", "config.json")
}

func (s Store) Load() (Config, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{Hosts: map[string]HostConfig{}}, nil
	}
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]HostConfig{}
	}
	return cfg, nil
}

func (s Store) Save(cfg Config) error {
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]HostConfig{}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, append(data, '\n'), 0o600)
}

func Resolve(cfg Config, env map[string]string, explicitURL string) (Resolved, error) {
	if env == nil {
		env = map[string]string{}
	}

	url := normalizeURL(explicitURL)
	if url == "" {
		url = normalizeURL(env["GITBUCKET_URL"])
	}
	if url == "" {
		url = normalizeURL(cfg.DefaultURL)
	}
	if url == "" {
		return Resolved{}, errors.New("GitBucket URL is required; set --url, GITBUCKET_URL, or run auth login")
	}

	host := HostConfig{}
	if cfg.Hosts != nil {
		host = cfg.Hosts[url]
	}

	if host.Legacy {
		if host.User == "" {
			return Resolved{}, errors.New("legacy GitBucket sign-in name is required; run auth login --legacy --user")
		}
		return Resolved{URL: url, Legacy: true, User: host.User}, nil
	}

	token := env["GITBUCKET_TOKEN"]
	if token == "" {
		token = host.Token
	}
	if token == "" {
		return Resolved{}, errors.New("GitBucket token is required; set --token, GITBUCKET_TOKEN, or run auth login")
	}

	return Resolved{URL: url, Token: token}, nil
}

func UpsertHost(cfg Config, url string, host HostConfig) Config {
	url = normalizeURL(url)
	if cfg.Hosts == nil {
		cfg.Hosts = map[string]HostConfig{}
	}
	cfg.DefaultURL = url
	cfg.Hosts[url] = host
	return cfg
}

func RemoveHost(cfg Config, url string) Config {
	url = normalizeURL(url)
	delete(cfg.Hosts, url)
	if cfg.DefaultURL == url {
		cfg.DefaultURL = ""
		for host := range cfg.Hosts {
			cfg.DefaultURL = host
			break
		}
	}
	return cfg
}

func NormalizeURL(url string) string {
	return normalizeURL(url)
}

func normalizeURL(url string) string {
	return strings.TrimRight(strings.TrimSpace(url), "/")
}
