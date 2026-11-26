package studio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
)

type providerImage struct {
	Output
	URL    string    `json:"url"`
	Base64 string    `json:"b64_json"`
	Format string    `json:"output_format"`
	Error  *APIError `json:"error"`
}

type providerResponse struct {
	Data      []providerImage `json:"data"`
	Usage     json.RawMessage `json:"usage"`
	Error     *APIError       `json:"error"`
	RequestID string          `json:"request_id"`
}

func normalizeOptions(options *Options) {
	if options.Count == 0 {
		options.Count = 1
	}
	if options.Resolution == "" {
		options.Resolution = "auto"
	}
	if options.Ratio == "" {
		options.Ratio = "auto"
	}
	if options.Background == "" {
		options.Background = "auto"
	}
	if options.Format == "" {
		options.Format = "png"
	}
	if options.MaxImages == 0 {
		options.MaxImages = 30
	}
}

func imageSize(options Options) (string, error) {
	if options.Resolution != "auto" && options.Resolution != "1K" && options.Resolution != "1.5K" && options.Resolution != "2K" {
		return "", fmt.Errorf("分辨率应为自动、1K、1.5K 或 2K")
	}
	if options.LayerDecomposition || options.Ratio == "auto" {
		if options.Resolution == "auto" {
			return "", nil
		}
		return options.Resolution, nil
	}
	parts := strings.Split(options.Ratio, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("比例格式应为宽:高")
	}
	width, errW := strconv.ParseFloat(parts[0], 64)
	height, errH := strconv.ParseFloat(parts[1], 64)
	ratio := width / height
	if errW != nil || errH != nil || width <= 0 || height <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 1.0/16 || ratio > 16 {
		return "", fmt.Errorf("Seedream 的宽高比范围为 1:16 至 16:1")
	}
	side := 2048.0
	if options.Resolution == "1K" {
		side = 1024
	} else if options.Resolution == "1.5K" {
		side = 1536
	}
	return fmt.Sprintf("%dx%d", int(math.Round(side*math.Sqrt(ratio))), int(math.Round(side/math.Sqrt(ratio)))), nil
}

func validateInput(input *GenerationInput) error {
	normalizeOptions(&input.Options)
	o := input.Options
	if o.Count < 1 {
		return fmt.Errorf("生成数量至少为 1")
	}
	if !o.LayerDecomposition && strings.TrimSpace(input.Prompt) == "" {
		return fmt.Errorf("请先描述你想生成或编辑的图片")
	}
	if len(input.ReferenceIDs) > 10 {
		return fmt.Errorf("Seedream 5.5 每次最多支持 10 张参考图")
	}
	if o.LayerDecomposition && len(input.ReferenceIDs) != 1 {
		return fmt.Errorf("图层拆分需要且只接受 1 张参考图")
	}
	if o.LayerDecomposition && (o.Count != 1 || o.MaxImages < 2 || o.MaxImages > 100) {
		return fmt.Errorf("图层拆分执行 1 次，最大输出图片数为 2–100（含底图）")
	}
	if o.Format != "png" && o.Format != "jpeg" {
		return fmt.Errorf("输出格式应为 PNG 或 JPEG")
	}
	if !o.LayerDecomposition && o.Background != "auto" && o.Background != "opaque" && o.Background != "transparent" {
		return fmt.Errorf("背景应为自动、不透明或透明")
	}
	if !o.LayerDecomposition && o.Background == "transparent" && o.Format == "jpeg" {
		return fmt.Errorf("透明背景需要选择 PNG 格式")
	}
	_, err := imageSize(o)
	return err
}

func requestBody(model Model, input GenerationInput, images []string) ([]byte, error) {
	body := map[string]any{
		"model": model.ModelName, "prompt": input.Prompt,
		"response_format": "b64_json", "output_format": input.Options.Format,
		"watermark": input.Options.Watermark,
	}
	if len(images) == 1 {
		body["image"] = images[0]
	} else if len(images) > 1 {
		body["image"] = images
	}
	size, err := imageSize(input.Options)
	if err != nil {
		return nil, err
	}
	if size != "" {
		body["size"] = size
	}
	if input.Options.LayerDecomposition {
		body["layer_decomposition"] = true
		body["layer_decomposition_options"] = map[string]any{
			"max_images": input.Options.MaxImages, "text_recognition": input.Options.TextRecognition,
		}
	} else if input.Options.Background != "auto" {
		body["background"] = input.Options.Background
	}
	return json.Marshal(body)
}

func callProvider(ctx context.Context, client *http.Client, model Model, body []byte) (*providerResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, model.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+model.APIKey)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	encoded, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("读取模型响应：%w", err)
	}
	requestID := response.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = response.Header.Get("X-Tt-Logid")
	}
	var result providerResponse
	decodeErr := json.Unmarshal(encoded, &result)
	if result.RequestID == "" {
		result.RequestID = requestID
	}
	if result.Error != nil {
		result.Error.Status = response.StatusCode
		result.Error.RequestID = result.RequestID
		return nil, result.Error
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || decodeErr != nil {
		return nil, &APIError{
			Message: "模型接口返回 " + response.Status,
			Status:  response.StatusCode, RequestID: requestID, Details: string(encoded),
		}
	}
	if len(result.Data) == 0 {
		return nil, &APIError{Message: "模型未返回图像", RequestID: result.RequestID, Details: string(encoded)}
	}
	return &result, nil
}

func imageBytes(ctx context.Context, client *http.Client, item providerImage) ([]byte, error) {
	if item.Error != nil {
		return nil, item.Error
	}
	if item.Base64 != "" {
		return base64.StdEncoding.DecodeString(item.Base64)
	}
	if item.URL == "" {
		return nil, fmt.Errorf("模型返回的图片缺少 b64_json 和 url")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("下载模型原图：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载模型原图：HTTP %d", response.StatusCode)
	}
	return io.ReadAll(response.Body)
}

func publicError(err error, secret string) *APIError {
	result := APIError{Message: err.Error()}
	if apiError, ok := err.(*APIError); ok {
		result = *apiError
	}
	if secret != "" {
		result.Message = strings.ReplaceAll(result.Message, secret, "[已隐藏密钥]")
		result.Details = strings.ReplaceAll(result.Details, secret, "[已隐藏密钥]")
	}
	return &result
}
