package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type agentWriterKey struct{}

func agentImageModel(model string) bool {
	m := strings.ToLower(strings.TrimPrefix(model, "models/"))
	return strings.HasPrefix(m, "gpt-image-") || strings.Contains(m, "nano-banana") || strings.Contains(m, "grok-imagine-image") || strings.Contains(m, "gemini-") && strings.Contains(m, "-image")
}

// Resolve public composite aliases after key allowlisting, before selecting a
// protocol. All calls still traverse the platform's real authorization gateway.
func (p *platformAgentExecutor) routeInvocation(c *gin.Context) {
	k, ok := middleware.GetAPIKeyFromContext(c)
	if !ok || k.Group == nil {
		c.AbortWithStatus(401)
		return
	}
	// Image multipart bodies are already consumed by the image gateway. Avoid
	// duplicating every reference into a second complete read-all buffer.
	if strings.HasPrefix(c.Request.URL.Path, "/v1/images/") && k.Group.Platform == service.PlatformOpenAI {
		if probe, _ := c.Request.Context().Value(agentProbeKey{}).(bool); probe {
			c.Status(204)
			return
		}
		p.openai.Images(c)
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	platform := k.Group.Platform
	if platform == service.PlatformComposite {
		resolver := p.openai.compositeResolver
		if resolver == nil {
			resolver = service.NewCompositeRouteResolver(nil)
		}
		decision, e := resolver.Resolve(c.Request.Context(), k.Group.ID, gjson.GetBytes(body, "model").String(), service.CompositeRouteEndpointResponses)
		if e != nil || !decision.Matched {
			c.AbortWithStatusJSON(400, gin.H{"code": "AGENT_MODEL_NOT_ALLOWED"})
			return
		}
		c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(c.Request.Context(), decision))
		platform = decision.TargetPlatform
		if decision.UpstreamModel != "" {
			body, err = sjson.SetBytes(body, "model", decision.UpstreamModel)
			if err != nil {
				c.AbortWithStatus(400)
				return
			}
		}
	}
	if platform == service.PlatformTypeSafe {
		c.AbortWithStatusJSON(400, gin.H{"code": "AGENT_TEXT_PROTOCOL_UNSUPPORTED"})
		return
	}
	if probe, _ := c.Request.Context().Value(agentProbeKey{}).(bool); probe {
		c.Status(204)
		return
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/images/") {
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		if platform != service.PlatformOpenAI {
			c.AbortWithStatusJSON(400, gin.H{"code": "AGENT_IMAGE_PROTOCOL_UNSUPPORTED"})
			return
		}
		p.openai.Images(c)
		return
	}
	effort, _ := c.Request.Context().Value(agentEffortKey{}).(string)
	forceAsk := false
	if trace := service.AgentInvocationFromContext(c.Request.Context()); trace != nil {
		forceAsk = trace.ForceAskUser
	}
	wireBody, chat, err := prepareAgentTextBody(body, platform, effort, forceAsk)
	if err != nil {
		c.AbortWithStatusJSON(400, gin.H{"code": "AGENT_PROTOCOL_CONVERSION_FAILED"})
		return
	}
	body = wireBody
	if chat {
		if w, ok := c.Request.Context().Value(agentWriterKey{}).(*agentResponseWriter); ok {
			w.chatState = apicompat.NewChatCompletionsToResponsesStreamState(gjson.GetBytes(body, "model").String())
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Request.URL.Path = "/v1/chat/completions"
		p.generic.ChatCompletions(c)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.ContentLength = int64(len(body))
	if domain.UsesOpenAIGateway(platform) {
		p.openai.Responses(c)
	} else {
		p.generic.Responses(c)
	}
}

type agentEffortKey struct{}

// Quote and dispatch share this exact transformation, including tool-choice
// restrictions, reasoning controls and Gemini's Chat Completions envelope.
func prepareAgentTextBody(body []byte, platform, effort string, forceAsk bool) ([]byte, bool, error) {
	if !domain.UsesOpenAIGateway(platform) {
		if effort == "" {
			effort = "medium"
		}
		var err error
		body, err = sjson.SetBytes(body, "reasoning", map[string]string{"effort": effort, "summary": "auto"})
		if err != nil {
			return nil, false, err
		}
		if forceAsk {
			body, err = sjson.SetBytes(body, "tool_choice", "auto")
			if err != nil {
				return nil, false, err
			}
		}
	}
	if platform != service.PlatformGemini {
		return body, false, nil
	}
	var req apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, false, err
	}
	chat, err := apicompat.ResponsesToChatCompletionsRequest(&req)
	if err != nil {
		return nil, false, err
	}
	chat.Stream = true
	chat.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: true}
	body, err = json.Marshal(chat)
	return body, true, err
}
