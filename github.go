package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

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

type Artifact struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	SizeInBytes        int64  `json:"size_in_bytes"`
	ArchiveDownloadURL string `json:"archive_download_url"`
}

func getToken() (string, error) {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return strings.TrimSpace(t), nil
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("未找到 token，请设置 GITHUB_TOKEN 或安装 gh 并登录")
	}
	return strings.TrimSpace(string(out)), nil
}

func getArtifact(repo, id, token string) (*Artifact, error) {
	u := fmt.Sprintf("https://api.github.com/repos/%s/actions/artifacts/%s", repo, id)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := getHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var art Artifact
	if err := json.NewDecoder(resp.Body).Decode(&art); err != nil {
		return nil, err
	}
	return &art, nil
}

func getRunArtifacts(repo, runID, token string) ([]Artifact, error) {
	u := fmt.Sprintf("https://api.github.com/repos/%s/actions/runs/%s/artifacts", repo, runID)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := getHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Artifacts []Artifact `json:"artifacts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Artifacts, nil
}

func resolveURL(apiURL, token string) (string, error) {
	var finalURL string
	client := &http.Client{
		Transport: getTransport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			finalURL = req.URL.String()
			return http.ErrUseLastResponse
		},
	}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	if finalURL == "" {
		return "", fmt.Errorf("未返回重定向地址")
	}
	return finalURL, nil
}

func resolveReleaseAssetURL(repo, tag, filename, token string) (string, int64, error) {
	u := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, tag)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := getHTTPClient().Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var release struct {
		Assets []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", 0, err
	}

	var assetID, assetSize int64
	for _, a := range release.Assets {
		if a.Name == filename {
			assetID = a.ID
			assetSize = a.Size
			break
		}
	}
	if assetID == 0 {
		return "", 0, fmt.Errorf("release %s 中没有文件 %q", tag, filename)
	}

	assetURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/assets/%d", repo, assetID)
	var finalURL string
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			finalURL = req.URL.String()
			return http.ErrUseLastResponse
		},
	}
	req2, _ := http.NewRequest("GET", assetURL, nil)
	req2.Header.Set("Authorization", "token "+token)
	req2.Header.Set("Accept", "application/octet-stream")

	resp2, err := client.Do(req2)
	if err != nil {
		return "", 0, err
	}
	resp2.Body.Close()
	if finalURL == "" {
		return "", 0, fmt.Errorf("未返回重定向地址")
	}
	return finalURL, assetSize, nil
}
