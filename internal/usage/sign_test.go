package usage

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAgentPlanSignature(t *testing.T) {
	request, err := http.NewRequest("POST", "https://ark.cn-beijing.volcengineapi.com/?Action=GetAFPUsage&Version=2024-01-01", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	// Fixed dummy credentials; expected value independently calculated from the documented HMAC steps.
	now := time.Date(2026, 9, 4, 18, 20, 30, 0, time.FixedZone("UTC+8", 8*60*60))
	signAgentPlan(request, []byte("{}"), Credentials{AccessKey: "fixture-access", SecretKey: "fixture-secret"}, now)
	expected := "HMAC-SHA256 Credential=fixture-access/20260904/cn-beijing/ark/request, SignedHeaders=content-type;host;x-content-sha256;x-date, Signature=1727f99e1bc6e1c14041ca641e6fa3a634cfa919ef194a706b6389800d3c4fa0"
	if request.Header.Get("Authorization") != expected || request.Header.Get("X-Date") != "20260904T102030Z" {
		t.Fatal("signature or UTC date does not match the reference")
	}
}
