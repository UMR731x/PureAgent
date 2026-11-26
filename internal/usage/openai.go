package usage

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func queryOpenAI(ctx context.Context, client *http.Client, model string, credentials Credentials, report *Report) {
	report.ConsoleURL = "https://platform.openai.com/settings/organization/billing/overview"
	report.Scope = "OpenAI 官方组织 · 当前模型 · 全部项目"
	report.RemainingNote = "OpenAI 官方 API 未提供账户剩余额度查询，请在官方账单页面查看余额。"
	report.Notices = []string{"按模型名称筛选本月图片用量，统计整个组织，可能存在入账延迟；不包含第三方中转用量。"}
	if credentials.AdminKey == "" {
		report.Errors = append(report.Errors, "请先配置 OpenAI Admin API Key；普通生图 API Key 无法查询组织用量。")
		return
	}
	now := report.CheckedAt
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	report.Period = start.Format("2006-01-02") + " 至 " + now.Format("2006-01-02 15:04") + "（UTC）"
	// Follow the API's default bucket size and pagination for the whole month.
	query := url.Values{"start_time": {strconv.FormatInt(start.Unix(), 10)}, "end_time": {strconv.FormatInt(now.Unix(), 10)},
		"bucket_width": {"1d"}, "models[]": {model}}
	var images, requests int64
	for {
		request, err := http.NewRequestWithContext(ctx, "GET", "https://api.openai.com/v1/organization/usage/images?"+query.Encode(), nil)
		if err != nil {
			report.Errors = append(report.Errors, err.Error())
			return
		}
		request.Header.Set("Authorization", "Bearer "+credentials.AdminKey)
		var response struct {
			Data []struct {
				Results []struct {
					Images   int64 `json:"images"`
					Requests int64 `json:"num_model_requests"`
				} `json:"results"`
			} `json:"data"`
			HasMore  bool   `json:"has_more"`
			NextPage string `json:"next_page"`
		}
		if err := requestJSON(client, request, &response); err != nil {
			report.Errors = append(report.Errors, err.Error())
			return
		}
		if response.Data == nil {
			report.Errors = append(report.Errors, "用量接口未返回 data，无法确认本月用量。")
			return
		}
		for _, bucket := range response.Data {
			for _, result := range bucket.Results {
				images += result.Images
				requests += result.Requests
			}
		}
		if !response.HasMore {
			break
		}
		if response.NextPage == "" || response.NextPage == query.Get("page") {
			report.Errors = append(report.Errors, fmt.Sprintf("用量接口分页异常，无法确认完整用量（游标 %q）。", response.NextPage))
			return
		}
		query.Set("page", response.NextPage)
	}
	report.Metrics = []Metric{{Name: "图片用量", Value: images, Unit: "张图片"}, {Name: "模型请求", Value: requests, Unit: "次"}}
}
