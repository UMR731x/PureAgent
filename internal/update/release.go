package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
)

const releaseEndpoint = "https://api.github.com/repos/UMR731x/PureAgent/releases/latest"

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}

type release struct {
	Tag    string         `json:"tag_name"`
	URL    string         `json:"html_url"`
	Assets []releaseAsset `json:"assets"`
}

func fetchRelease(ctx context.Context, client *http.Client) (release, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseEndpoint, nil)
	if err != nil {
		return release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "PureAgent")
	response, err := client.Do(request)
	if err != nil {
		return release{}, fmt.Errorf("无法连接 GitHub：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return release{}, fmt.Errorf("GitHub 尚未发布正式版本")
	}
	if response.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("GitHub 返回 %s，请稍后重新检查", response.Status)
	}
	var latest release
	err = json.NewDecoder(response.Body).Decode(&latest)
	return latest, err
}

func newerVersion(latest, current string) (bool, error) {
	parse := func(version string) ([3]int, error) {
		var numbers [3]int
		parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
		if len(parts) != len(numbers) {
			return numbers, fmt.Errorf("无法识别版本号 %q", version)
		}
		for i, part := range parts {
			number, err := strconv.Atoi(part)
			if err != nil || number < 0 {
				return numbers, fmt.Errorf("无法识别版本号 %q", version)
			}
			numbers[i] = number
		}
		return numbers, nil
	}
	newNumbers, err := parse(latest)
	if err != nil {
		return false, err
	}
	if current == "dev" {
		return true, nil
	}
	oldNumbers, err := parse(current)
	if err != nil {
		return false, err
	}
	for i := range newNumbers {
		if newNumbers[i] != oldNumbers[i] {
			return newNumbers[i] > oldNumbers[i], nil
		}
	}
	return false, nil
}

func (r release) platformAsset() (releaseAsset, error) {
	name := "PureAgent-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("此版本没有适用于 %s/%s 的客户端", runtime.GOOS, runtime.GOARCH)
}

func download(ctx context.Context, client *http.Client, asset releaseAsset, path string, mode os.FileMode) error {
	if !strings.HasPrefix(asset.Digest, "sha256:") {
		return fmt.Errorf("GitHub 未提供客户端的 SHA-256 校验值，无法确认下载完整性")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("下载客户端失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载客户端失败：%s", response.Status)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, digest), response.Body); err != nil {
		return err
	}
	if "sha256:"+hex.EncodeToString(digest.Sum(nil)) != strings.ToLower(asset.Digest) {
		return fmt.Errorf("下载的客户端校验不一致，已保留当前版本，请重新检查更新")
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return file.Close()
}
