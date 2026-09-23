package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

func (a *App) emit(name string, data map[string]interface{}) {
	runtime.EventsEmit(a.ctx, name, data)
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

func (a *App) Download(input string) error {
	input = strings.TrimSpace(input)
	p, err := parseGitHubURL(input)
	if err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	dlDir := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(dlDir, 0755); err != nil {
		return err
	}

	ctx := context.Background()
	emit := a.emit

	switch p.Kind {
	case "release-asset":
		token, err := getToken()
		if err != nil {
			return err
		}
		repo := p.Owner + "/" + p.Repo
		cdnURL, size, err := resolveReleaseAssetURL(repo, p.Tag, p.Filename, token)
		if err != nil {
			return err
		}
		outPath := filepath.Join(dlDir, p.Filename)
		return downloadMulti(ctx, emit, cdnURL, outPath, size, func() (string, int64, error) {
			return resolveReleaseAssetURL(repo, p.Tag, p.Filename, token)
		})

	case "artifact":
		token, err := getToken()
		if err != nil {
			return err
		}
		repo := p.Owner + "/" + p.Repo
		art, err := getArtifact(repo, p.ArtifactID, token)
		if err != nil {
			return err
		}
		cdnURL, err := resolveURL(art.ArchiveDownloadURL, token)
		if err != nil {
			return err
		}
		outPath := filepath.Join(dlDir, art.Name+".zip")
		return downloadMulti(ctx, emit, cdnURL, outPath, art.SizeInBytes, func() (string, int64, error) {
			cdn, err := resolveURL(art.ArchiveDownloadURL, token)
			return cdn, art.SizeInBytes, err
		})

	case "run":
		token, err := getToken()
		if err != nil {
			return err
		}
		repo := p.Owner + "/" + p.Repo
		arts, err := getRunArtifacts(repo, p.RunID, token)
		if err != nil {
			return err
		}
		if len(arts) == 0 {
			return fmt.Errorf("该 run 没有 artifact")
		}
		for i := range arts {
			art := &arts[i]
			a.emit("download:file", map[string]interface{}{
				"index": i, "total": len(arts), "name": art.Name,
			})
			cdnURL, err := resolveURL(art.ArchiveDownloadURL, token)
			if err != nil {
				return err
			}
			outPath := filepath.Join(dlDir, art.Name+".zip")
			err = downloadMulti(ctx, emit, cdnURL, outPath, art.SizeInBytes, func() (string, int64, error) {
				cdn, err := resolveURL(art.ArchiveDownloadURL, token)
				return cdn, art.SizeInBytes, err
			})
			if err != nil {
				return err
			}
		}
		a.emit("download:done", map[string]interface{}{"dir": dlDir})
		return nil
	}
	return fmt.Errorf("当前不支持下载类型: %s", p.Kind)
}
