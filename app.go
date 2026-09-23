package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
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

// Parse 解析用户输入，返回可读结果
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
		// /{owner}/{repo}/releases/download/{tag}/{filename}
		if len(parts) >= 6 && parts[2] == "releases" && parts[3] == "download" {
			return &parsedURL{
				Kind:     "release-asset",
				Owner:    parts[0],
				Repo:     parts[1],
				Tag:      parts[4],
				Filename: strings.Join(parts[5:], "/"),
			}, nil
		}
		// /{owner}/{repo}/actions/runs/{run_id}
		if len(parts) >= 5 && parts[2] == "actions" && parts[3] == "runs" {
			return &parsedURL{
				Kind:  "run",
				Owner: parts[0],
				Repo:  parts[1],
				RunID: parts[4],
			}, nil
		}
	case "api.github.com":
		// /repos/{owner}/{repo}/actions/artifacts/{id}/zip
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
