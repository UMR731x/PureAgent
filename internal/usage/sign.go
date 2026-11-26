package usage

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AgentPlan management uses Volcengine's AK/SK signature, not the image API key.
func signAgentPlan(request *http.Request, body []byte, credentials Credentials, now time.Time) {
	stamp := now.UTC().Format("20060102T150405Z")
	date := stamp[:8]
	payloadHash := fmt.Sprintf("%x", sha256.Sum256(body))
	signedHeaders := "content-type;host;x-content-sha256;x-date"
	headers := "content-type:application/json\nhost:" + request.URL.Host + "\nx-content-sha256:" + payloadHash + "\nx-date:" + stamp + "\n"
	canonical := strings.Join([]string{request.Method, "/", request.URL.RawQuery, headers, signedHeaders, payloadHash}, "\n")
	scope := date + "/cn-beijing/ark/request"
	toSign := "HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + fmt.Sprintf("%x", sha256.Sum256([]byte(canonical)))
	key := []byte(credentials.SecretKey)
	for _, part := range []string{date, "cn-beijing", "ark", "request", toSign} {
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(part))
		key = mac.Sum(nil)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Date", stamp)
	request.Header.Set("X-Content-Sha256", payloadHash)
	request.Header.Set("Authorization", fmt.Sprintf("HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%x", credentials.AccessKey, scope, signedHeaders, key))
}
