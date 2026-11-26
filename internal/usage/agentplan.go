package usage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type afpWindow struct {
	Quota, Used string
	ResetTime   int64
}

type afpQuota struct {
	SeatID, PlanType     string
	AFPDaily, AFPMonthly *afpWindow
}

type planDetail struct {
	ObjectName, BillingType, Unit string
	Usage                         int64
}

type planResult struct {
	afpQuota
	SeatAFPUsages    []afpQuota
	Details          []planDetail
	SeatUsageDetails []struct {
		SeatID  string
		Details []planDetail
	}
}

func queryAgentPlan(ctx context.Context, client *http.Client, model string, credentials Credentials, report *Report) {
	report.ConsoleURL = "https://ark.volcengine.com/region:cn-beijing/subscription/agent-plan"
	report.Scope = "AgentPlan 套餐共享额度 · 图片使用近一天、近一月窗口"
	report.Notices = []string{"AFP 为套餐共享额度，不是当前模型独享，也不能直接换算为图片张数。", "模型明细按北京时间统计本月，可能延迟半天至一天；不包含第三方中转用量。"}
	if credentials.AccessKey == "" || credentials.SecretKey == "" {
		report.Errors = append(report.Errors, "请先配置火山引擎 Access Key / Secret Key；生图 API Key 无法查询套餐额度。")
		return
	}
	quotaAction, detailAction := "GetAFPUsage", "GetUsageDetails"
	quotaBody := map[string]any{}
	now := report.CheckedAt.In(time.FixedZone("UTC+8", 8*60*60))
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("2006-01-02")
	end := now.Format("2006-01-02")
	report.Period = start + " 至 " + end + "（北京时间）"
	detailBody := map[string]any{"QueryInterval": "Day", "Filter": map[string]any{"StartTime": start, "EndTime": end, "ObjectName": []string{model}}}
	if credentials.SeatID != "" {
		quotaAction, detailAction = "GetSeatAFPUsage", "GetSeatUsageDetails"
		quotaBody["SeatIDs"] = []string{credentials.SeatID}
		detailBody["SeatIDs"] = []string{credentials.SeatID}
		report.Scope = "AgentPlan 企业席位共享额度 · 图片使用近一天、近一月窗口"
	}
	result, err := requestAgentPlan(ctx, client, credentials, quotaAction, quotaBody)
	if err == nil {
		err = collectQuota(result, credentials.SeatID, report)
	}
	if err != nil {
		report.Errors = append(report.Errors, "套餐额度："+err.Error())
	}
	result, err = requestAgentPlan(ctx, client, credentials, detailAction, detailBody)
	if err == nil {
		err = collectDetails(result, model, credentials.SeatID, report)
	}
	if err != nil {
		report.Errors = append(report.Errors, "模型用量："+err.Error())
	}
}

func requestAgentPlan(ctx context.Context, client *http.Client, credentials Credentials, action string, input any) (*planResult, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	query := url.Values{"Action": {action}, "Version": {"2024-01-01"}}
	request, err := http.NewRequestWithContext(ctx, "POST", "https://ark.cn-beijing.volcengineapi.com/?"+query.Encode(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	signAgentPlan(request, body, credentials, time.Now())
	var response struct {
		ResponseMetadata struct {
			RequestID string
			Error     *struct{ Code, Message string }
		}
		Result *planResult
	}
	if err := requestJSON(client, request, &response); err != nil {
		return nil, err
	}
	if apiError := response.ResponseMetadata.Error; apiError != nil {
		return nil, fmt.Errorf("%s：%s（Request ID：%s）", apiError.Code, apiError.Message, response.ResponseMetadata.RequestID)
	}
	if response.Result == nil {
		return nil, fmt.Errorf("接口未返回 Result")
	}
	return response.Result, nil
}

func collectQuota(result *planResult, seat string, report *Report) error {
	quota := &result.afpQuota
	if seat != "" {
		quota = nil
		for i := range result.SeatAFPUsages {
			if result.SeatAFPUsages[i].SeatID == seat {
				quota = &result.SeatAFPUsages[i]
				break
			}
		}
		if quota == nil {
			return fmt.Errorf("接口未返回该席位的额度，请检查 Seat ID")
		}
	}
	report.Plan = quota.PlanType
	for _, item := range []struct {
		name   string
		window *afpWindow
	}{{"近一天", quota.AFPDaily}, {"近一月", quota.AFPMonthly}} {
		if item.window == nil {
			continue
		}
		total, totalErr := strconv.ParseFloat(item.window.Quota, 64)
		used, usedErr := strconv.ParseFloat(item.window.Used, 64)
		if totalErr != nil || usedErr != nil || math.IsNaN(total) || math.IsInf(total, 0) || math.IsNaN(used) || math.IsInf(used, 0) {
			return fmt.Errorf("%s的 AFP 数值无法读取", item.name)
		}
		report.Quotas = append(report.Quotas, Window{Name: item.name, Quota: total, Used: used, Remaining: total - used, ResetTime: item.window.ResetTime})
	}
	if len(report.Quotas) == 0 {
		return fmt.Errorf("接口未提供适用于图片的日/月额度")
	}
	return nil
}

func collectDetails(result *planResult, model, seat string, report *Report) error {
	details := result.Details
	if seat != "" {
		found := false
		for _, entry := range result.SeatUsageDetails {
			if entry.SeatID == seat {
				details, found = entry.Details, true
				break
			}
		}
		if !found {
			return fmt.Errorf("接口未返回该席位的模型明细")
		}
	}
	for _, detail := range details {
		if detail.ObjectName != model {
			continue
		}
		name := detail.BillingType
		switch name {
		case "WithinPlan":
			name = "套餐内用量"
		case "OutsideOfPlan":
			name = "套餐外用量"
		}
		unit := detail.Unit
		if unit == "Images" {
			unit = "张图片"
		}
		found := false
		for i := range report.Metrics {
			metric := &report.Metrics[i]
			if metric.Name == name && metric.Unit == unit {
				metric.Value += detail.Usage
				found = true
				break
			}
		}
		if !found {
			report.Metrics = append(report.Metrics, Metric{Name: name, Unit: unit, Value: detail.Usage})
		}
	}
	if len(report.Metrics) == 0 {
		report.Notices = append(report.Notices, "该时段暂无当前模型的用量明细；明细入账有延迟。")
	}
	return nil
}
