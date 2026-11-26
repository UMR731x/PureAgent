package studio

import (
	"encoding/json"
	"time"

	"pureagent/internal/usage"
)

type Model struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	ModelName string             `json:"model_name"`
	Provider  string             `json:"provider,omitempty"`
	URL       string             `json:"url"`
	APIKey    string             `json:"api_key,omitempty"`
	HasKey    bool               `json:"has_key,omitempty"`
	Usage     *usage.Credentials `json:"usage,omitempty"`
}

type Config struct {
	Models []Model `json:"models"`
}

type Asset struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Name   string `json:"name"`
	MIME   string `json:"mime"`
	Bytes  int64  `json:"bytes"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Kind   string `json:"kind"`
}

type Options struct {
	Count              int    `json:"count"`
	Resolution         string `json:"resolution"`
	Ratio              string `json:"ratio"`
	Format             string `json:"format"`
	Background         string `json:"background"`
	Watermark          bool   `json:"watermark"`
	LayerDecomposition bool   `json:"layer_decomposition"`
	MaxImages          int    `json:"max_images"`
	TextRecognition    bool   `json:"text_recognition"`
	Size               string `json:"size,omitempty"`
	Quality            string `json:"quality,omitempty"`
	Complexity         string `json:"complexity,omitempty"`
}

type GenerationInput struct {
	ModelID      string   `json:"model_id"`
	Prompt       string   `json:"prompt"`
	ReferenceIDs []string `json:"reference_ids"`
	MarkupID     string   `json:"markup_id,omitempty"`
	Options      Options  `json:"options"`
}

type APIError struct {
	Message   string `json:"message"`
	Code      string `json:"code,omitempty"`
	Status    int    `json:"http_status,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Details   string `json:"details,omitempty"`
}

func (e *APIError) Error() string { return e.Message }

type Output struct {
	AssetID         string          `json:"asset_id"`
	Name            string          `json:"name,omitempty"`
	Description     string          `json:"description,omitempty"`
	Size            string          `json:"size,omitempty"`
	Background      string          `json:"background,omitempty"`
	ZIndex          *int            `json:"z_index,omitempty"`
	BoundingBox     json.RawMessage `json:"bounding_box,omitempty"`
	TextRecognition json.RawMessage `json:"text_recognition,omitempty"`
}

type Turn struct {
	ID string `json:"id"`
	GenerationInput
	ModelName  string            `json:"model_name"`
	Status     string            `json:"status"`
	CreatedAt  time.Time         `json:"created_at"`
	FinishedAt *time.Time        `json:"finished_at,omitempty"`
	Completed  int               `json:"completed"`
	Outputs    []Output          `json:"outputs"`
	Usage      []json.RawMessage `json:"usage,omitempty"`
	RequestIDs []string          `json:"request_ids,omitempty"`
	Error      *APIError         `json:"error,omitempty"`
}

type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Assets    []Asset   `json:"assets"`
	Turns     []Turn    `json:"turns"`
}

type SessionSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updated_at"`
	Running   bool      `json:"running"`
	Thumbnail string    `json:"thumbnail,omitempty"`
}
