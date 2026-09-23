package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

const partsCount = 8
const minSizeForMulti = 1 * 1024 * 1024

type emitFn func(name string, data map[string]interface{})

// downloadMulti 多线程下载，支持断点续传和链接过期恢复
func downloadMulti(ctx context.Context, emit emitFn, initialURL, outPath string, total int64, resolveFn func() (string, int64, error)) error {
	// 已完成则跳过
	if fi, err := os.Stat(outPath); err == nil && total > 0 && fi.Size() == total {
		emit("download:progress", map[string]interface{}{
			"downloaded": total, "total": total, "speed": 0,
			"file": outPath, "done": true,
		})
		return nil
	}

	// 小文件或 total 未知直接单线程
	if total > 0 && total < minSizeForMulti {
		return downloadSingle(ctx, emit, initialURL, outPath, total)
	}

	// 探测 Range
	if !supportsRange(initialURL) {
		return downloadSingle(ctx, emit, initialURL, outPath, total)
	}

	if total <= 0 {
		total = headContentLength(initialURL)
		if total <= 0 {
			return downloadSingle(ctx, emit, initialURL, outPath, 0)
		}
	}

	chunk := (total + partsCount - 1) / partsCount

	var urlMu sync.Mutex
	currentURL := initialURL
	getURL := func() string {
		urlMu.Lock()
		defer urlMu.Unlock()
		return currentURL
	}
	refreshURL := func() error {
		urlMu.Lock()
		defer urlMu.Unlock()
		newURL, _, err := resolveFn()
		if err != nil {
			return err
		}
		currentURL = newURL
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, partsCount)
	doneCh := make(chan struct{})

	go monitorProgress(emit, outPath, total, doneCh)

	actualParts := 0
	for i := 0; i < partsCount; i++ {
		start := int64(i) * chunk
		if start >= total {
			break
		}
		end := start + chunk - 1
		if end >= total {
			end = total - 1
		}
		actualParts = i + 1

		wg.Add(1)
		go func(idx int, s, e int64) {
			defer wg.Done()
			partFile := fmt.Sprintf("%s.part%d", outPath, idx)
			if err := downloadPart(ctx, getURL, refreshURL, s, e, partFile); err != nil {
				errCh <- fmt.Errorf("分片 %d: %w", idx, err)
			}
		}(i, start, end)
	}

	wg.Wait()
	close(doneCh)

	select {
	case err := <-errCh:
		return err
	default:
	}

	return mergeParts(outPath, actualParts)
}

func downloadSingle(ctx context.Context, emit emitFn, rawURL, outPath string, total int64) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	resp, err := getHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if total <= 0 {
		total = resp.ContentLength
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var downloaded int64
	start := time.Now()
	lastEmit := time.Now()
	buf := make([]byte, 64*1024)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := resp.Body.Read(buf)
		if n > 0 {
			f.Write(buf[:n])
			downloaded += int64(n)
			if time.Since(lastEmit) > 150*time.Millisecond {
				speed := float64(downloaded) / time.Since(start).Seconds()
				emit("download:progress", map[string]interface{}{
					"downloaded": downloaded, "total": total,
					"speed": speed, "file": outPath, "done": false,
				})
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
	emit("download:progress", map[string]interface{}{
		"downloaded": downloaded, "total": total,
		"speed": 0, "file": outPath, "done": true,
	})
	return nil
}

func downloadPart(ctx context.Context, getURL func() string, refreshURL func() error, start, end int64, partFile string) error {
	expected := end - start + 1

	for attempt := 0; attempt < 3; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var have int64
		if fi, err := os.Stat(partFile); err == nil {
			have = fi.Size()
			if have > expected {
				os.Remove(partFile)
				have = 0
			}
			if have == expected {
				return nil
			}
		}

		reqStart := start + have
		req, _ := http.NewRequestWithContext(ctx, "GET", getURL(), nil)
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", reqStart, end))

		resp, err := getHTTPClient().Do(req)
		if err != nil {
			if attempt < 2 {
				time.Sleep(time.Second)
				continue
			}
			return err
		}

		if resp.StatusCode == 403 || resp.StatusCode == 401 {
			resp.Body.Close()
			if err := refreshURL(); err != nil {
				return fmt.Errorf("刷新链接失败: %w", err)
			}
			continue
		}
		if resp.StatusCode != 206 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}

		f, err := os.OpenFile(partFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			resp.Body.Close()
			return err
		}
		_, err = io.Copy(f, resp.Body)
		f.Close()
		resp.Body.Close()
		if err != nil {
			if attempt < 2 {
				time.Sleep(time.Second)
				continue
			}
			return err
		}

		if fi, err := os.Stat(partFile); err == nil && fi.Size() == expected {
			return nil
		}
	}
	return fmt.Errorf("重试 3 次后仍未完成")
}

func supportsRange(url string) bool {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", "bytes=0-0")
	resp, err := getHTTPClient().Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == 206
}

func headContentLength(url string) int64 {
	resp, err := http.Head(url)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.ContentLength
}

func mergeParts(outPath string, parts int) error {
	for i := 0; i < parts; i++ {
		partFile := fmt.Sprintf("%s.part%d", outPath, i)
		if _, err := os.Stat(partFile); err != nil {
			return fmt.Errorf("缺少分片 %s", partFile)
		}
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := 0; i < parts; i++ {
		partFile := fmt.Sprintf("%s.part%d", outPath, i)
		pf, err := os.Open(partFile)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, pf)
		pf.Close()
		if err != nil {
			return err
		}
		os.Remove(partFile)
	}
	return nil
}

func monitorProgress(emit emitFn, outPath string, total int64, done <-chan struct{}) {
	start := time.Now()

	// 记录启动时已经存在的分片大小，续传时用于扣除
	var initial int64
	for i := 0; i < partsCount; i++ {
		if fi, err := os.Stat(fmt.Sprintf("%s.part%d", outPath, i)); err == nil {
			initial += fi.Size()
		}
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			var sum int64
			for i := 0; i < partsCount; i++ {
				if fi, err := os.Stat(fmt.Sprintf("%s.part%d", outPath, i)); err == nil {
					sum += fi.Size()
				}
			}
			// 只计算本次运行新增的部分
			delta := sum - initial
			if delta < 0 {
				delta = 0
			}
			speed := float64(delta) / time.Since(start).Seconds()

			emit("download:progress", map[string]interface{}{
				"downloaded": sum, "total": total,
				"speed": speed, "file": outPath, "done": false,
			})
		}
	}
}
