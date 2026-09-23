package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type configData struct {
	Proxy string `json:"proxy"`
}

var (
	configMu      sync.RWMutex
	currentConfig configData
	configPath    string

	httpClientMu sync.RWMutex
	httpClient   = &http.Client{}
)

func initConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".ghfast-gui")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	configPath = filepath.Join(dir, "config.json")

	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &currentConfig)
	}

	// 代理优先级：配置文件 > 环境变量 > 直连
	proxy := currentConfig.Proxy
	if proxy == "" {
		proxy = os.Getenv("HTTPS_PROXY")
	}
	if proxy == "" {
		proxy = os.Getenv("HTTP_PROXY")
	}

	return rebuildHTTPClient(proxy)
}

func getConfig() configData {
	configMu.RLock()
	defer configMu.RUnlock()
	return currentConfig
}

func setProxy(proxy string) error {
	proxy = strings.TrimSpace(proxy)

	// 空字符串表示清除代理
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return fmt.Errorf("代理地址无效: %w", err)
		}
		if u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("代理地址格式应为 http://host:port")
		}
	}

	if err := rebuildHTTPClient(proxy); err != nil {
		return err
	}

	configMu.Lock()
	currentConfig.Proxy = proxy
	data, _ := json.MarshalIndent(currentConfig, "", "  ")
	configMu.Unlock()

	return os.WriteFile(configPath, data, 0644)
}

func getHTTPClient() *http.Client {
	httpClientMu.RLock()
	defer httpClientMu.RUnlock()
	return httpClient
}

func getTransport() http.RoundTripper {
	httpClientMu.RLock()
	defer httpClientMu.RUnlock()
	return httpClient.Transport
}

func rebuildHTTPClient(proxy string) error {
	if proxy == "" {
		httpClientMu.Lock()
		httpClient = &http.Client{}
		httpClientMu.Unlock()
		return nil
	}

	u, err := url.Parse(proxy)
	if err != nil {
		return fmt.Errorf("代理地址无效: %w", err)
	}

	httpClientMu.Lock()
	httpClient = &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(u)},
	}
	httpClientMu.Unlock()
	return nil
}
