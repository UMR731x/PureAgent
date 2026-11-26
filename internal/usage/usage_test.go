package usage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonReply(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestAgentPlanPersonalAndSeatUsage(t *testing.T) {
	for _, seat := range []string{"", "seat-1"} {
		t.Run("seat="+seat, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "ark.cn-beijing.volcengineapi.com" || r.Method != "POST" || !strings.Contains(r.Header.Get("Authorization"), "/cn-beijing/ark/request") {
					t.Fatal("incorrect management endpoint or authentication")
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if seat != "" && body["SeatIDs"].([]any)[0] != seat {
					t.Fatal("seat not selected")
				}
				action := r.URL.Query().Get("Action")
				quota := `{"PlanType":"Large","AFPDaily":{"Quota":"100","Used":"22.5","ResetTime":1778803200000},"AFPMonthly":{"Quota":"2000","Used":"850.5"}}`
				details := `[{"ObjectName":"doubao-seedream-5.5","BillingType":"WithinPlan","Unit":"Images","Usage":2},{"ObjectName":"doubao-seedream-5.5","BillingType":"WithinPlan","Unit":"Images","Usage":3},{"ObjectName":"another-model","BillingType":"WithinPlan","Unit":"Images","Usage":99},{"ObjectName":"doubao-seedream-5.5","BillingType":"OutsideOfPlan","Unit":"Tokens","Usage":100}]`
				if strings.Contains(action, "UsageDetails") {
					filter := body["Filter"].(map[string]any)
					if filter["ObjectName"].([]any)[0] != "doubao-seedream-5.5" || filter["StartTime"] == "" || filter["EndTime"] == "" {
						t.Fatal("model/date filter missing")
					}
					if seat != "" {
						return jsonReply(`{"Result":{"SeatUsageDetails":[{"SeatID":"seat-1","Details":` + details + `}]}}`), nil
					}
					return jsonReply(`{"Result":{"Details":` + details + `}}`), nil
				}
				if seat != "" {
					return jsonReply(`{"Result":{"SeatAFPUsages":[` + strings.Replace(quota, `{"PlanType"`, `{"SeatID":"seat-1","PlanType"`, 1) + `]}}`), nil
				}
				return jsonReply(`{"Result":` + quota + `}`), nil
			})}
			report := Query(context.Background(), client, "seedream", "doubao-seedream-5.5", Credentials{AccessKey: "test-ak", SecretKey: "test-sk", SeatID: seat})
			if calls != 2 || len(report.Errors) != 0 || len(report.Quotas) != 2 {
				t.Fatalf("query failed: %+v", report)
			}
			if report.Quotas[0].Remaining != 77.5 || report.Quotas[1].Remaining != 1149.5 {
				t.Fatal("incorrect shared quota")
			}
			if len(report.Metrics) != 2 || report.Metrics[0].Value != 5 || report.Metrics[1].Value != 100 {
				t.Fatal("model, units or billing categories were mixed")
			}
		})
	}
}

func TestAgentPlanPartialErrorAndRedaction(t *testing.T) {
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("Action") == "GetAFPUsage" {
			return jsonReply(`{"Result":{"AFPDaily":{"Quota":"0","Used":"0"}}}`), nil
		}
		return jsonReply(`{"ResponseMetadata":{"RequestId":"request-1","Error":{"Code":"Denied","Message":"test-sk denied"}}}`), nil
	})}
	report := Query(context.Background(), client, "", "doubao-seedream-5.5", Credentials{AccessKey: "test-ak", SecretKey: "test-sk"})
	if len(report.Quotas) != 1 || report.Quotas[0].Remaining != 0 || len(report.Errors) != 1 || len(report.Metrics) != 0 {
		t.Fatalf("partial data lost or invented: %+v", report)
	}
	if strings.Contains(report.Errors[0], "test-sk") || !strings.Contains(report.Errors[0], "request-1") {
		t.Fatal("error redaction lost request context")
	}
}

func TestOpenAIModelPaginationAndUnknownBalance(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "api.openai.com" || r.URL.Path != "/v1/organization/usage/images" || r.Header.Get("Authorization") != "Bearer test-admin" {
			t.Fatal("incorrect OpenAI usage authentication")
		}
		if r.URL.Query().Get("models[]") != "gpt-image-2.6-flare" || r.URL.Query().Get("start_time") == "" {
			t.Fatal("missing model/time filter")
		}
		if calls == 1 {
			return jsonReply(`{"data":[{"results":[{"images":2,"num_model_requests":1}]}],"has_more":true,"next_page":"page-two"}`), nil
		}
		if r.URL.Query().Get("page") != "page-two" {
			t.Fatal("missing pagination cursor")
		}
		return jsonReply(`{"data":[{"results":[{"images":3,"num_model_requests":2}]}],"has_more":false,"next_page":null}`), nil
	})}
	report := Query(context.Background(), client, "openai", "gpt-image-2.6-flare", Credentials{AdminKey: "test-admin"})
	if calls != 2 || len(report.Errors) != 0 || len(report.Metrics) != 2 || report.Metrics[0].Value != 5 || report.Metrics[1].Value != 3 {
		t.Fatalf("incorrect OpenAI usage: %+v", report)
	}
	if report.RemainingNote == "" || len(report.Quotas) != 0 {
		t.Fatal("unknown balance reported as a quota")
	}
}

func TestUsageMissingCredentialsAndInvalidResponses(t *testing.T) {
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) { return jsonReply(`{}`), nil })}
	for _, provider := range []string{"seedream", "openai"} {
		missing := Query(context.Background(), client, provider, "model", Credentials{})
		if len(missing.Errors) == 0 || len(missing.Quotas)+len(missing.Metrics) != 0 {
			t.Fatal("missing credentials reported as zero usage")
		}
		invalid := Query(context.Background(), client, provider, "model", Credentials{AccessKey: "test-ak", SecretKey: "test-sk", AdminKey: "test-admin"})
		if len(invalid.Errors) == 0 || len(invalid.Quotas)+len(invalid.Metrics) != 0 {
			t.Fatal("invalid response reported as zero usage")
		}
	}
}
