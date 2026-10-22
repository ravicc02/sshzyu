package handler

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var agentErrorURL = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)
var agentErrorBearer = regexp.MustCompile(`(?i)bearer\s+[^\s,;"']+`)
var agentErrorSecretField = regexp.MustCompile(`(?i)(["']?(?:api[-_ ]?key|authorization|password|secret|(?:access[-_ ]?|refresh[-_ ]?)?token|cookie)["']?\s*[:=]\s*)(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;]+)`)
var agentErrorKeyToken = regexp.MustCompile(`(?i)\b(?:sk-|AIza)[A-Za-z0-9_-]{5,}\b`)
var agentErrorOpaqueToken = regexp.MustCompile(`[A-Za-z0-9_./+=-]{32,}`)

func agentFailureMessage(message string, secrets ...string) string {
	if len(message) > 16<<10 {
		message = strings.ToValidUTF8(message[:16<<10], "")
	}
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	message = service.SanitizeUpstreamErrorMessage(message)
	message = agentErrorURL.ReplaceAllString(message, "[upstream]")
	message = agentErrorBearer.ReplaceAllString(message, "Bearer [redacted]")
	message = agentErrorSecretField.ReplaceAllString(message, "$1[redacted]")
	message = agentErrorKeyToken.ReplaceAllString(message, "[redacted]")
	message = agentErrorOpaqueToken.ReplaceAllString(message, "[redacted]")
	message = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, message)
	text := []rune(strings.TrimSpace(message))
	if len(text) > 1024 {
		text = append(text[:1024], '…')
	}
	return string(text)
}

// Capture only the final failed attempt. Successful failover must not inherit a
// rejected attempt's status or error; account names, URLs and credentials are
// never part of this public projection.
func agentFailureMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		writer, _ := c.Request.Context().Value(agentWriterKey{}).(*agentResponseWriter)
		if writer == nil || writer.code < 400 {
			return
		}
		// A later 403 cannot prove an earlier failed-over request was rejected.
		// Keep ambiguous attempts unsettled while still returning the final error.
		if value, ok := c.Get(service.OpsUpstreamErrorsKey); ok {
			if attempts, ok := value.([]*service.OpsUpstreamErrorEvent); ok {
				for _, attempt := range attempts {
					if attempt != nil && !knownAgentRejection(attempt.UpstreamStatusCode) {
						writer.ambiguousFailureAttempt = true
					}
				}
			}
		}
		status := c.GetInt(service.OpsUpstreamStatusCodeKey)
		message := strings.TrimSpace(c.GetString(service.OpsUpstreamErrorMessageKey))
		if status < 400 || status > 599 {
			status = 0
		}
		if message == "" {
			return
		}
		secret := strings.TrimSpace(strings.TrimPrefix(c.Request.Header.Get("Authorization"), "Bearer "))
		writer.failure = &agent.FailureDetail{StatusCode: writer.code, UpstreamStatusCode: status, Message: agentFailureMessage(message, secret)}
	}
}

func agentErrorBodyMessage(body []byte) string {
	if len(body) > 16<<10 {
		body = body[:16<<10]
	}
	for _, path := range []string{"error.message", "message", "error", "detail", "msg", "incomplete_details.reason"} {
		value := gjson.GetBytes(body, path)
		if value.Type != gjson.String {
			continue
		}
		message := strings.TrimSpace(value.String())
		if strings.HasPrefix(message, "{") {
			if inner := gjson.Get(message, "error.message"); inner.Type == gjson.String {
				message = inner.String()
			}
		}
		if message != "" {
			return message
		}
	}
	return ""
}
func knownAgentRejection(status int) bool {
	switch status {
	case 400, 401, 402, 403, 404, 405, 406, 410, 413, 415, 422, 429, 451:
		return true
	}
	return false
}
func agentHTTPFailure(writer *agentResponseWriter, dispatched bool, secret string) *agent.InvocationFailure {
	detail := writer.failure
	if detail == nil {
		detail = &agent.FailureDetail{StatusCode: writer.code, Message: agentErrorBodyMessage(writer.body.Bytes())}
	} else {
		copy := *detail
		detail = &copy
	}
	if detail.Message == "" {
		detail.Message = fmt.Sprintf("上游接口返回 HTTP %d", writer.code)
	}
	detail.Message = agentFailureMessage(detail.Message, secret)
	status := detail.UpstreamStatusCode
	if status == 0 {
		status = detail.StatusCode
	}
	return &agent.InvocationFailure{Code: "AGENT_UPSTREAM_REJECTED", Dispatched: dispatched, ResultKnown: !dispatched || (knownAgentRejection(status) && !writer.ambiguousFailureAttempt), Detail: detail}
}

func agentIncompleteFailure(code, message string, dispatched bool) *agent.InvocationFailure {
	return &agent.InvocationFailure{Code: code, Dispatched: dispatched, Detail: &agent.FailureDetail{Message: agentFailureMessage(message)}}
}
