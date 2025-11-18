package utils

import (
	"fmt"
	"net/http"
	"net/url"

	"go-telegram-forwarder-bot/internal/config"
)

// CreateHTTPClientWithProxy creates an HTTP client with proxy support
func CreateHTTPClientWithProxy(cfg *config.ProxyConfig) (*http.Client, error) {
	client := &http.Client{}

	if !cfg.Enabled {
		return client, nil
	}

	proxyURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	// Set proxy authentication if provided
	if cfg.Username != "" || cfg.Password != "" {
		proxyURL.User = url.UserPassword(cfg.Username, cfg.Password)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}

	client.Transport = transport
	return client, nil
}
