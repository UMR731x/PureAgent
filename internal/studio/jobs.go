package studio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func findModel(config Config, id string) (Model, error) {
	for _, model := range config.Models {
		if model.ID == id {
			if model.APIKey == "" {
				return Model{}, fmt.Errorf("请先在设置中保存此模型的 API Key")
			}
			return model, nil
		}
	}
	return Model{}, fmt.Errorf("请选择已配置的模型")
}

func validateReferences(session *Session, input GenerationInput) error {
	for _, id := range input.ReferenceIDs {
		asset, found := assetByID(session, id)
		if !found {
			return fmt.Errorf("参考图不属于当前任务或不存在")
		}
		if asset.Bytes > maxReferenceBytes {
			return fmt.Errorf("参考图 %s 超过 API 的 30 MB 限制", asset.Name)
		}
		if input.Options.LayerDecomposition && asset.MIME != "image/png" && asset.MIME != "image/jpeg" {
			return fmt.Errorf("图层拆分仅支持 PNG 或 JPEG 原图")
		}
	}
	return nil
}

func (a *App) startGeneration(w http.ResponseWriter, r *http.Request) {
	var input GenerationInput
	if !decodeRequest(w, r, &input) {
		return
	}
	if err := validateInput(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if a.updating {
		writeError(w, http.StatusConflict, fmt.Errorf("正在更新客户端，请在重启后开始新生成；现有任务会继续完成"))
		return
	}
	session, err := a.store.session(r.PathValue("session"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if _, running := a.jobs[session.ID]; running || a.ctx.Err() != nil {
		writeError(w, http.StatusConflict, fmt.Errorf("当前任务正在生成或程序正在退出"))
		return
	}
	config, err := a.store.config()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	model, err := findModel(config, input.ModelID)
	if err == nil {
		err = validateReferences(session, input)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	turn := Turn{ID: newID(), GenerationInput: input, ModelName: model.ModelName,
		Status: "running", CreatedAt: time.Now().UTC(), Outputs: []Output{}}
	if len(session.Turns) == 0 {
		session.Title = strings.TrimSpace(input.Prompt)
		if session.Title == "" {
			session.Title = "图层拆分"
		}
	}
	session.Turns = append(session.Turns, turn)
	if err := a.store.saveSession(session); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.jobs[session.ID] = cancel
	a.wg.Add(1)
	go a.runGeneration(ctx, session, turn, model)
	writeResponse(w, http.StatusAccepted, session)
}

func (a *App) referenceImages(session *Session, input GenerationInput) ([]string, error) {
	directory, _ := a.store.sessionPath(session.ID)
	images := make([]string, 0, len(input.ReferenceIDs))
	for _, id := range input.ReferenceIDs {
		asset, _ := assetByID(session, id)
		original, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(asset.Path)))
		if err != nil {
			return nil, fmt.Errorf("读取参考图 %s：%w", asset.Name, err)
		}
		images = append(images, "data:"+asset.MIME+";base64,"+base64.StdEncoding.EncodeToString(original))
	}
	return images, nil
}

func (a *App) updateTurn(sessionID, turnID string, update func(*Session, *Turn)) error {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	session, err := a.store.session(sessionID)
	if err != nil {
		return err
	}
	for i := range session.Turns {
		if session.Turns[i].ID == turnID {
			update(session, &session.Turns[i])
			return a.store.saveSession(session)
		}
	}
	return fmt.Errorf("生成记录不存在")
}

func (a *App) runGeneration(ctx context.Context, session *Session, turn Turn, model Model) {
	defer a.wg.Done()
	images, generationErr := a.referenceImages(session, turn.GenerationInput)
	var body []byte
	if generationErr == nil {
		body, generationErr = requestBody(model, turn.GenerationInput, images)
	}
	for i := 0; i < turn.Options.Count && generationErr == nil; i++ {
		var result *providerResponse
		result, generationErr = callProvider(ctx, a.client, model, body)
		if generationErr == nil {
			generationErr = a.saveResult(ctx, session.ID, turn.ID, i+1, result, turn.Options.Format)
		}
	}
	status := "completed"
	var failure *APIError
	if generationErr != nil {
		status, failure = "failed", publicError(generationErr, model.APIKey)
	}
	if ctx.Err() != nil {
		status = "cancelled"
		failure = &APIError{Message: "已停止本地生成任务。上游可能仍在处理或已计费；已保存的图片已保留。"}
	}
	err := a.updateTurn(session.ID, turn.ID, func(_ *Session, saved *Turn) {
		now := time.Now().UTC()
		saved.Status, saved.Error, saved.FinishedAt = status, failure, &now
	})
	if err != nil {
		log.Printf("保存任务状态失败 (%s)：%v", session.ID, err)
	}
	a.store.mu.Lock()
	a.jobs[session.ID]()
	delete(a.jobs, session.ID)
	a.store.mu.Unlock()
}

func (a *App) saveResult(ctx context.Context, sessionID, turnID string, batch int, result *providerResponse, format string) error {
	for i, item := range result.Data {
		original, err := imageBytes(ctx, a.client, item)
		if err != nil {
			return err
		}
		if item.Format != "" {
			format = item.Format
		}
		name := fmt.Sprintf("pureagent-%s-%d-%d.%s", turnID, batch, i+1, format)
		asset, err := a.store.writeAsset(sessionID, name, "generated", original)
		if err != nil {
			return fmt.Errorf("保存原图：%w", err)
		}
		output := item.Output
		output.AssetID = asset.ID
		if err := a.updateTurn(sessionID, turnID, func(session *Session, saved *Turn) {
			session.Assets = append(session.Assets, asset)
			saved.Outputs = append(saved.Outputs, output)
		}); err != nil {
			return err
		}
	}
	return a.updateTurn(sessionID, turnID, func(_ *Session, saved *Turn) {
		saved.Completed++
		if len(result.Usage) > 0 && string(result.Usage) != "null" {
			saved.Usage = append(saved.Usage, json.RawMessage(result.Usage))
		}
		if result.RequestID != "" {
			saved.RequestIDs = append(saved.RequestIDs, result.RequestID)
		}
	})
}

func (a *App) cancelGeneration(w http.ResponseWriter, r *http.Request) {
	a.store.mu.Lock()
	defer a.store.mu.Unlock()
	if cancel, found := a.jobs[r.PathValue("session")]; found {
		cancel()
	}
	writeResponse(w, http.StatusOK, map[string]bool{"cancelled": true})
}
