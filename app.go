package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) Parse(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("输入为空")
	}

	p, err := parseGitHubURL(input)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("类型: %s\n仓库: %s/%s\nTag: %s\n文件: %s\nRunID: %s\nArtifactID: %s",
		p.Kind, p.Owner, p.Repo, p.Tag, p.Filename, p.RunID, p.ArtifactID), nil
}

// Download 下载指定 URL，通过事件推送进度
func (a *App) Download(rawURL string) error {
	p, err := parseGitHubURL(rawURL)
	if err != nil {
		return err
	}
	if p.Kind != "release-asset" {
		return fmt.Errorf("当前只支持 Release 文件直链下载，其他类型后续支持")
	}

	filename := p.Filename
	home, _ := os.UserHomeDir()
	outPath := filepath.Join(home, "Downloads", filename)

	req, _ := http.NewRequest("GET", rawURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var downloaded int64
	start := time.Now()
	lastEmit := time.Now()
	buf := make([]byte, 64*1024)

	emit := func(done bool) {
		speed := float64(downloaded) / time.Since(start).Seconds()
		runtime.EventsEmit(a.ctx, "download:progress", map[string]interface{}{
			"downloaded": downloaded,
			"total":      total,
			"speed":      speed,
			"file":       outPath,
			"done":       done,
		})
	}

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			f.Write(buf[:n])
			downloaded += int64(n)
			if time.Since(lastEmit) > 200*time.Millisecond {
				emit(false)
				lastEmit = time.Now()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	emit(true)
	return nil
}

type parsedURL struct {
	Kind       string
	Owner      string
	Repo       string
	Tag        string
	Filename   string
	RunID      string
	ArtifactID string
}

func parseGitHubURL(raw string) (*parsedURL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("链接格式无效: %w", err)
	}
	path := strings.TrimPrefix(u.Path, "/")
	parts := strings.Split(path, "/")

	switch u.Host {
	case "github.com", "www.github.com":
		if len(parts) >= 6 && parts[2] == "releases" && parts[3] == "download" {
			return &parsedURL{
				Kind:     "release-asset",
				Owner:    parts[0],
				Repo:     parts[1],
				Tag:      parts[4],
				Filename: strings.Join(parts[5:], "/"),
			}, nil
		}
		if len(parts) >= 5 && parts[2] == "actions" && parts[3] == "runs" {
			return &parsedURL{
				Kind:  "run",
				Owner: parts[0],
				Repo:  parts[1],
				RunID: parts[4],
			}, nil
		}
	case "api.github.com":
		if len(parts) >= 6 && parts[0] == "repos" && parts[3] == "actions" && parts[4] == "artifacts" {
			return &parsedURL{
				Kind:       "artifact",
				Owner:      parts[1],
				Repo:       parts[2],
				ArtifactID: parts[5],
			}, nil
		}
	}
	return nil, fmt.Errorf("不支持的链接: %s", raw)
}
