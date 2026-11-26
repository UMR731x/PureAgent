package studio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const openAIReferenceBytes = 50 * 1024 * 1024
const openAIMarkupPixels = 16777216

func validateOpenAIInput(input *GenerationInput) error {
	normalizeOptions(&input.Options)
	options := &input.Options
	if options.Size == "" {
		options.Size = "auto"
	}
	if options.Quality == "" {
		options.Quality = "auto"
	}
	if options.Count < 1 || strings.TrimSpace(input.Prompt) == "" {
		return fmt.Errorf("请填写图像描述，并选择至少 1 张图片")
	}
	if len(input.ReferenceIDs) > 16 {
		return fmt.Errorf("OpenAI Images 每次最多支持 16 张参考图")
	}
	if options.LayerDecomposition {
		return fmt.Errorf("OpenAI Images 不提供图层拆分与结构化 OCR，请切换到 Seedream")
	}
	if options.Watermark {
		return fmt.Errorf("OpenAI Images 没有 Seedream 的水印开关，请关闭此选项")
	}
	if !slices.Contains([]string{"png", "jpeg", "webp"}, options.Format) {
		return fmt.Errorf("OpenAI 图片输出请选择 PNG、JPEG 或 WebP")
	}
	if !slices.Contains([]string{"auto", "opaque", "transparent"}, options.Background) {
		return fmt.Errorf("背景应为自动、不透明或透明")
	}
	if options.Background == "transparent" && options.Format == "jpeg" {
		return fmt.Errorf("透明背景需要选择 PNG 或 WebP")
	}
	if !slices.Contains([]string{"auto", "low", "medium", "high", "xhigh", "max"}, options.Quality) {
		return fmt.Errorf("图像质量应为自动、低、中、高、超高或最高")
	}
	if !slices.Contains([]string{"", "low", "medium", "high"}, options.Complexity) {
		return fmt.Errorf("复杂度应为默认、低、中或高")
	}
	return validateOpenAISize(options.Size)
}

func validateOpenAISize(size string) error {
	if size == "auto" {
		return nil
	}
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return fmt.Errorf("OpenAI 图像尺寸应为自动或宽x高，例如 1536x1024")
	}
	width, errW := strconv.Atoi(parts[0])
	height, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil || width <= 0 || height <= 0 || width > 3840 || height > 3840 || width%16 != 0 || height%16 != 0 {
		return fmt.Errorf("OpenAI 图像宽高需为 16 的正整数倍，单边不超过 3840 像素")
	}
	if width > height*3 || height > width*3 || width*height < 655360 || width*height > 8294400 {
		return fmt.Errorf("OpenAI 宽高比需在 1:3 至 3:1 之间，总像素为 655,360–8,294,400")
	}
	return nil
}

func validateOpenAIReferences(session *Session, input GenerationInput) error {
	for _, id := range input.ReferenceIDs {
		asset, found := assetByID(session, id)
		if !found {
			return fmt.Errorf("参考图不属于当前任务或不存在")
		}
		if asset.Bytes >= openAIReferenceBytes || !slices.Contains([]string{"image/png", "image/jpeg", "image/webp"}, asset.MIME) {
			return fmt.Errorf("OpenAI 参考图 %s 需为 PNG、JPEG 或 WebP，且小于 50 MiB", asset.Name)
		}
	}
	if input.MarkupID == "" {
		return nil
	}
	if len(input.ReferenceIDs) == 0 {
		return fmt.Errorf("局部标记需要第一张参考图作为编辑底图")
	}
	markup, found := assetByID(session, input.MarkupID)
	if !found || markup.MIME != "image/png" || markup.Bytes >= openAIReferenceBytes {
		return fmt.Errorf("局部标记需要本任务中小于 50 MiB 的透明 PNG")
	}
	first, _ := assetByID(session, input.ReferenceIDs[0])
	if int64(markup.Width)*int64(markup.Height) > openAIMarkupPixels || int64(first.Width)*int64(first.Height) > openAIMarkupPixels {
		return fmt.Errorf("OpenAI 局部标记与编辑底图均不能超过 16,777,216 像素")
	}
	return nil
}

func openAIEndpoint(configured string, editing bool) (string, error) {
	endpoint, err := url.Parse(configured)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(endpoint.Path, "/")
	for _, suffix := range []string{"/images/generations", "/images/edits", "/images"} {
		if strings.HasSuffix(path, suffix) {
			path = strings.TrimSuffix(path, suffix)
			break
		}
	}
	if path == "" {
		path = "/v1"
	}
	endpoint.Path = path + "/images/generations"
	if editing {
		endpoint.Path = path + "/images/edits"
	}
	return endpoint.String(), nil
}

func (a *App) openAIRequest(session *Session, input GenerationInput, model Model) (generationRequest, error) {
	endpoint, err := openAIEndpoint(model.URL, len(input.ReferenceIDs) > 0)
	if err != nil {
		return generationRequest{}, err
	}
	fields := map[string]string{
		"model": model.ModelName, "prompt": input.Prompt, "size": input.Options.Size,
		"quality": input.Options.Quality, "output_format": input.Options.Format, "background": input.Options.Background,
	}
	if input.Options.Complexity != "" {
		fields["complexity"] = input.Options.Complexity
	}
	if len(input.ReferenceIDs) == 0 {
		body, err := json.Marshal(fields)
		return generationRequest{URL: endpoint, ContentType: "application/json", Body: body}, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return generationRequest{}, err
		}
	}
	for _, id := range input.ReferenceIDs {
		if err := a.writeOpenAIImage(writer, "image[]", session, id); err != nil {
			return generationRequest{}, err
		}
	}
	if input.MarkupID != "" {
		if err := a.writeOpenAIImage(writer, "markup", session, input.MarkupID); err != nil {
			return generationRequest{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return generationRequest{}, err
	}
	return generationRequest{URL: endpoint, ContentType: writer.FormDataContentType(), Body: body.Bytes()}, nil
}

func (a *App) writeOpenAIImage(writer *multipart.Writer, field string, session *Session, id string) error {
	asset, _ := assetByID(session, id) // References were checked before the job started.
	directory, _ := a.store.sessionPath(session.ID)
	file, err := os.Open(filepath.Join(directory, filepath.FromSlash(asset.Path)))
	if err != nil {
		return fmt.Errorf("读取原图 %s：%w", asset.Name, err)
	}
	defer file.Close()
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": asset.Name}))
	header.Set("Content-Type", asset.MIME)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}
