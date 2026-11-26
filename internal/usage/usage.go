package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Credentials struct {
	AccessKey    string `json:"access_key,omitempty"`
	SecretKey    string `json:"secret_key,omitempty"`
	AdminKey     string `json:"admin_key,omitempty"`
	SeatID       string `json:"seat_id,omitempty"`
	HasAccessKey bool   `json:"has_access_key,omitempty"`
	HasSecretKey bool   `json:"has_secret_key,omitempty"`
	HasAdminKey  bool   `json:"has_admin_key,omitempty"`
}

func (c Credentials) Redacted() Credentials {
	return Credentials{SeatID: c.SeatID, HasAccessKey: c.AccessKey != "", HasSecretKey: c.SecretKey != "", HasAdminKey: c.AdminKey != ""}
}

func (c Credentials) Merge(previous Credentials) Credentials {
	merged := Credentials{AccessKey: strings.TrimSpace(c.AccessKey), SecretKey: strings.TrimSpace(c.SecretKey), AdminKey: strings.TrimSpace(c.AdminKey), SeatID: strings.TrimSpace(c.SeatID)}
	if merged.AccessKey == "" {
		merged.AccessKey = previous.AccessKey
	}
	if merged.SecretKey == "" {
		merged.SecretKey = previous.SecretKey
	}
	if merged.AdminKey == "" {
		merged.AdminKey = previous.AdminKey
	}
	return merged
}

type Window struct {
	Name      string  `json:"name"`
	Quota     float64 `json:"quota"`
	Used      float64 `json:"used"`
	Remaining float64 `json:"remaining"`
	ResetTime int64   `json:"reset_time"`
}

type Metric struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
	Unit  string `json:"unit"`
}

type Report struct {
	ModelName     string    `json:"model_name"`
	Period        string    `json:"period"`
	Scope         string    `json:"scope"`
	Plan          string    `json:"plan,omitempty"`
	Quotas        []Window  `json:"quotas"`
	Metrics       []Metric  `json:"metrics"`
	Notices       []string  `json:"notices"`
	Errors        []string  `json:"errors"`
	RemainingNote string    `json:"remaining_note,omitempty"`
	ConsoleURL    string    `json:"console_url"`
	CheckedAt     time.Time `json:"checked_at"`
}

func Query(ctx context.Context, client *http.Client, provider, model string, credentials Credentials) Report {
	report := Report{ModelName: model, CheckedAt: time.Now().UTC()}
	if provider == "openai" {
		queryOpenAI(ctx, client, model, credentials, &report)
	} else {
		queryAgentPlan(ctx, client, model, credentials, &report)
	}
	for i, message := range report.Errors {
		for _, secret := range []string{credentials.AccessKey, credentials.SecretKey, credentials.AdminKey} {
			if secret != "" {
				message = strings.ReplaceAll(message, secret, "[已隐藏]")
			}
		}
		report.Errors[i] = message
	}
	return report
}

func requestJSON(client *http.Client, request *http.Request, value any) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d：%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, value); err != nil {
		return fmt.Errorf("用量接口返回了无法读取的数据：%w", err)
	}
	return nil
}
