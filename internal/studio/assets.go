package studio

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Seedream's documented per-reference upload limit; originals are never resized.
const maxReferenceBytes = 30 * 1024 * 1024

var imageExtensions = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".webp": "image/webp", ".bmp": "image/bmp", ".tif": "image/tiff",
	".tiff": "image/tiff", ".gif": "image/gif", ".heic": "image/heic", ".heif": "image/heif",
}

func assetByID(session *Session, id string) (Asset, bool) {
	for _, asset := range session.Assets {
		if asset.ID == id {
			return asset, true
		}
	}
	return Asset{}, false
}

func (s *Store) writeAsset(sessionID, name, kind string, original []byte) (Asset, error) {
	extension := strings.ToLower(filepath.Ext(name))
	mediaType := http.DetectContentType(original)
	if mediaType == "application/octet-stream" {
		mediaType = imageExtensions[extension]
	}
	if !strings.HasPrefix(mediaType, "image/") {
		return Asset{}, fmt.Errorf("附件需要是 Seedream 支持的图片格式")
	}
	if imageExtensions[extension] == "" {
		for candidate, candidateMIME := range imageExtensions {
			if candidateMIME == mediaType {
				extension = candidate
				break
			}
		}
	}
	if extension == "" {
		return Asset{}, fmt.Errorf("无法识别模型返回的图片格式：%s", mediaType)
	}
	asset := Asset{ID: newID(), Name: name, MIME: mediaType, Bytes: int64(len(original)), Kind: kind}
	asset.Path = "sources/" + asset.ID + extension
	if dimensions, _, err := image.DecodeConfig(bytes.NewReader(original)); err == nil {
		asset.Width, asset.Height = dimensions.Width, dimensions.Height
	}
	directory, err := s.sessionPath(sessionID)
	if err != nil {
		return Asset{}, err
	}
	if err := os.WriteFile(filepath.Join(directory, filepath.FromSlash(asset.Path)), original, 0600); err != nil {
		return Asset{}, err
	}
	return asset, nil
}

func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	part, err := reader.NextPart()
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("请选择图片文件"))
		return
	}
	defer part.Close()
	name := part.FileName()
	if imageExtensions[strings.ToLower(filepath.Ext(name))] == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("支持 PNG、JPEG、WebP、BMP、TIFF、GIF、HEIC、HEIF 图片"))
		return
	}
	original, err := io.ReadAll(io.LimitReader(part, maxReferenceBytes+1))
	if err != nil || len(original) > maxReferenceBytes {
		writeError(w, http.StatusBadRequest, fmt.Errorf("读取附件失败，单张图片不能超过 30 MB"))
		return
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	session, err := a.store.session(r.PathValue("session"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	asset, err := a.store.writeAsset(session.ID, name, "upload", original)
	if err == nil {
		session.Assets = append(session.Assets, asset)
		err = a.store.saveSession(session)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeResponse(w, http.StatusCreated, asset)
}

func (a *App) serveAsset(w http.ResponseWriter, r *http.Request) {
	a.store.mu.Lock()
	session, err := a.store.session(r.PathValue("session"))
	a.store.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	asset, found := assetByID(session, r.PathValue("asset"))
	if !found {
		writeError(w, http.StatusNotFound, fmt.Errorf("图片不存在"))
		return
	}
	directory, _ := a.store.sessionPath(session.ID)
	w.Header().Set("Content-Type", asset.MIME)
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": asset.Name}))
	}
	http.ServeFile(w, r, filepath.Join(directory, filepath.FromSlash(asset.Path)))
}
