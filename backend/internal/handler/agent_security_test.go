//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/agent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type securityKeyRepo struct {
	service.APIKeyRepository
	keys []service.APIKey
}

func (r securityKeyRepo) GetByID(_ context.Context, id int64) (*service.APIKey, error) {
	for _, key := range r.keys {
		if key.ID == id {
			return &key, nil
		}
	}
	return nil, agent.ErrNotFound
}
func (r securityKeyRepo) ListByUserID(_ context.Context, _ int64, _ pagination.PaginationParams, _ service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	return r.keys, &pagination.PaginationResult{}, nil
}
func securityKeys() []service.APIKey {
	result := []service.APIKey{}
	for i, platform := range []string{service.PlatformOpenAI, service.PlatformDeepseek} {
		gid := int64(i + 10)
		result = append(result, service.APIKey{ID: int64(i + 1), UserID: 1, Status: service.StatusActive, User: &service.User{ID: 1, Status: service.StatusActive}, GroupID: &gid, Group: &service.Group{ID: gid, Platform: platform, Name: platform, Status: service.StatusActive, AllowImageGeneration: true}})
	}
	return result
}
func TestAgentSecurityDSKeysAreTextOnlyAndKeepOwnership(t *testing.T) {
	cfg := &config.Config{}
	p := &platformAgentExecutor{keys: service.NewAPIKeyService(securityKeyRepo{keys: securityKeys()}, nil, nil, nil, nil, nil, cfg)}
	text, err := p.Options(context.Background(), 1, "text")
	require.NoError(t, err)
	require.Len(t, text, 2)
	image, err := p.Options(context.Background(), 1, "image")
	require.NoError(t, err)
	require.Len(t, image, 1)
	require.Equal(t, int64(1), image[0].ID)
	_, err = p.key(context.Background(), agent.Binding{KeyID: 2, UserID: 2})
	require.ErrorIs(t, err, agent.ErrNotFound)
	_, err = p.key(context.Background(), agent.Binding{KeyID: 2, UserID: 1})
	require.NoError(t, err)
}
func TestAgentSecurityExecuteRetainsChargeOnBrokenStream(t *testing.T) {
	for _, code := range []int{200, 502} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			cfg := &config.Config{}
			p := &platformAgentExecutor{keys: service.NewAPIKeyService(securityKeyRepo{keys: securityKeys()}, nil, nil, nil, nil, nil, cfg)}
			p.engine = gin.New()
			p.engine.POST("/v1/responses", func(c *gin.Context) {
				trace := service.AgentInvocationFromContext(c.Request.Context())
				trace.MarkDispatched()
				service.RecordAgentCharge(c.Request.Context(), .01)
				c.Header("Content-Type", "text/event-stream")
				c.Status(code)
				_, _ = io.WriteString(c.Writer, "data: {broken\n\n")
			})
			out, err := p.Execute(context.Background(), agent.Invocation{OperationID: "op-test", Kind: "text", Model: "gpt-6.1-sol", Binding: agent.Binding{UserID: 1, KeyID: 1}})
			require.Error(t, err)
			require.Equal(t, "0.01000000", out.Cost)
			require.False(t, out.BillingPending)
			require.True(t, out.Unknown)
		})
	}
}

type partialImageSecurityUpstream struct {
	service.HTTPUpstream
	calls int
}

func (u *partialImageSecurityUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	u.calls++
	body := `data: {"type":"image_generation.completed","model":"gpt-image-2","size":"1024x1024","b64_json":"aGVsbG8=","usage":{"input_tokens":10,"output_tokens":20}}` + "\n\n" + `data: {"type":"error","error":{"type":"server_error","message":"overloaded"}}` + "\n\n"
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(bytes.NewBufferString(body))}, nil
}

type securityBillingRepo struct {
	service.UsageBillingRepository
	commands []*service.UsageBillingCommand
}

func (r *securityBillingRepo) Apply(_ context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	r.commands = append(r.commands, cmd)
	return &service.UsageBillingApplyResult{Applied: false}, nil
}
func TestAgentSecurityPartialImageErrorStillRecordsUsageWithoutFailover(t *testing.T) {
	cfg := &config.Config{RunMode: config.RunModeStandard}
	cfg.Default.RateMultiplier = 1
	gid := int64(3130)
	price := .17
	group := &service.Group{ID: gid, Platform: service.PlatformOpenAI, RateMultiplier: 1, ImageRateMultiplier: 1, AllowImageGeneration: true, ImagePrice1K: &price, ImagePrice2K: &price, ImagePrice4K: &price}
	accountRepo := openAIImagesFailoverAccountRepo{accounts: []service.Account{{ID: 1, Name: "image-account", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Credentials: map[string]any{"access_token": "test-token"}}}}
	upstream := &partialImageSecurityUpstream{}
	billingRepo := &securityBillingRepo{}
	balance := service.NewBillingCacheService(newHandlerInflightCache(100), nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(balance.Stop)
	gw := service.NewOpenAIGatewayService(accountRepo, nil, billingRepo, nil, nil, nil, nil, cfg, nil, nil, service.NewBillingService(cfg, nil), nil, balance, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	h := NewOpenAIGatewayHandler(gw, service.NewConcurrencyService(nil), balance, service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)
	trace := &service.AgentInvocationTrace{OperationID: "partial-image-op", RunID: "run-test"}
	ctx := service.WithAgentInvocation(context.Background(), trace)
	req := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewBufferString(`{"model":"gpt-image-2","prompt":"cat","stream":true}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 99, GroupID: &gid, Group: group, User: &service.User{ID: 100}})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 100, Concurrency: 0})
	h.Images(c)
	require.Equal(t, 1, upstream.calls)
	require.Len(t, billingRepo.commands, 1, "partial result must reach mandatory billing")
	require.Equal(t, 1, billingRepo.commands[0].ImageCount)
	require.Equal(t, "agent:partial-image-op", billingRepo.commands[0].RequestID)
	cost, recorded := trace.Charge()
	require.True(t, recorded)
	require.InDelta(t, .17, cost, .00000001)
}

func TestAgentAllRegisteredPlatformsAndCompositeKeyDiscovery(t *testing.T) {
	keys := []service.APIKey{}
	for i, platform := range append(domain.ConcretePlatformIDs(), service.PlatformComposite) {
		gid := int64(i + 1)
		keys = append(keys, service.APIKey{ID: gid, UserID: 1, Status: service.StatusActive, User: &service.User{ID: 1, Status: service.StatusActive}, GroupID: &gid, Group: &service.Group{ID: gid, Name: platform, Platform: platform, Status: service.StatusActive, AllowImageGeneration: true}})
	}
	p := &platformAgentExecutor{keys: service.NewAPIKeyService(securityKeyRepo{keys: keys}, nil, nil, nil, nil, nil, &config.Config{})}
	options, err := p.Options(context.Background(), 1, "text")
	require.NoError(t, err)
	require.Len(t, options, len(keys))
	for _, option := range options {
		_, err := p.key(context.Background(), agent.Binding{UserID: 1, KeyID: option.ID})
		require.NoError(t, err, option.Platform)
		if option.Platform == service.PlatformTypeSafe {
			require.NotEmpty(t, option.DisabledReason)
		} else {
			require.Empty(t, option.DisabledReason, option.Platform)
		}
	}
}
func TestAgentGenericGatewayBillingIsInlineAndKeepsPrivateTrace(t *testing.T) {
	h := &GatewayHandler{}
	trace := &service.AgentInvocationTrace{OperationID: "generic-op"}
	ctx := service.WithAgentInvocation(context.Background(), trace)
	for _, submit := range []func(context.Context, service.UsageRecordTask){h.submitUsageRecordTask, h.submitMandatoryUsageRecordTask} {
		called := false
		submit(ctx, func(taskCtx context.Context) {
			require.Same(t, trace, service.AgentInvocationFromContext(taskCtx))
			service.RecordAgentCharge(taskCtx, .01)
			called = true
		})
		require.True(t, called)
	}
	cost, recorded := trace.Charge()
	require.True(t, recorded)
	require.InDelta(t, .02, cost, .0000001)
}

func TestAgentCompositeAliasesResolveBeforeProtocolDispatch(t *testing.T) {
	for _, platform := range domain.ConcretePlatformIDs() {
		t.Run(platform, func(t *testing.T) {
			resolver := service.NewCompositeRouteResolver(&compositeWSRouteRepo{routes: []service.CompositeModelRoute{{Enabled: true, PublicModel: "public-alias", MatchType: service.CompositeRouteMatchExact, TargetPlatform: platform, Endpoint: service.CompositeRouteEndpointResponses, UpstreamModel: "real-model"}}})
			p := &platformAgentExecutor{openai: &OpenAIGatewayHandler{compositeResolver: resolver}}
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("POST", "/v1/responses", bytes.NewBufferString(`{"model":"public-alias","input":"test"}`)).WithContext(context.WithValue(context.Background(), agentProbeKey{}, true))
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{Group: &service.Group{ID: 1, Platform: service.PlatformComposite}})
			p.routeInvocation(c)
			c.Writer.WriteHeaderNow()
			if platform == service.PlatformTypeSafe {
				require.Equal(t, 400, rec.Code)
			} else {
				require.Equal(t, 204, rec.Code)
			}
			target, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
			require.True(t, ok)
			require.Equal(t, platform, target)
		})
	}
}

func TestAgentNativeQuoteUsesConvertedWireBody(t *testing.T) {
	cfg := &config.Config{}
	input, output := .000001, .000005
	for _, platform := range []string{service.PlatformAnthropic, service.PlatformGemini, service.PlatformAntigravity} {
		t.Run(platform, func(t *testing.T) {
			keys := securityKeys()[:1]
			key := &keys[0]
			key.Group.Platform = platform
			key.Group.RateMultiplier = 1
			key.Group.ModelPricing = []service.ChannelModelPricing{{Platform: platform, Models: []string{"native-model"}, BillingMode: service.BillingModeToken, InputPrice: &input, OutputPrice: &output}}
			billing := service.NewBillingService(cfg, nil)
			resolver := service.NewModelPricingResolver(nil, billing)
			gateway := service.NewGatewayService(nil, nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, billing, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, resolver, nil, nil, nil)
			p := &platformAgentExecutor{keys: service.NewAPIKeyService(securityKeyRepo{keys: keys}, nil, nil, nil, nil, nil, cfg), gateway: gateway}
			in := agent.Invocation{Kind: "text", Model: "native-model", ReasoningEffort: "high", Tools: true, ImageTools: true, ForceAskUser: true, Input: []json.RawMessage{agent.UserItem("先问我")}, Binding: agent.Binding{UserID: 1, KeyID: key.ID}}
			quote, err := p.Quote(context.Background(), in)
			require.NoError(t, err)
			_, _, original, err := p.payload(in)
			require.NoError(t, err)
			wire, chat, err := prepareAgentTextBody(original, platform, in.ReasoningEffort, true)
			require.NoError(t, err)
			if chat {
				var req apicompat.ChatCompletionsRequest
				require.NoError(t, json.Unmarshal(wire, &req))
				require.Len(t, req.Tools, 1)
				require.Equal(t, "ask_user", req.Tools[0].Function.Name)
			}
			trace := &service.AgentInvocationTrace{CostLimit: quote}
			ctx := service.WithAgentInvocation(context.Background(), trace)
			require.NoError(t, gateway.CheckAgentSelectedAccountPrice(ctx, key, &service.Account{Platform: platform}, in.Model, in.Model, "text", len(wire)), "quote and selected-account checks must use the same serialization")
		})
	}
}
