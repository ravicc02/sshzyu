package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrHostedOpenAIAccountNotFound = infraerrors.NotFound("HOSTED_OPENAI_ACCOUNT_NOT_FOUND", "hosted OpenAI account not found")

type HostedOpenAIAccountRecord struct {
	ID                   int64
	UserID               int64
	AccountID            int64
	GroupID              int64
	SubscriptionID       *int64
	ProxyID              *int64
	Model                string
	SupportsMessages     bool
	Status               string
	AccountEmail         string
	PlanType             string
	LastQuotaRefreshedAt *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	UserName             string
	UserEmail            string
	GroupName            string
	ProxyName            string
}

type HostedOpenAIAccountRepository interface {
	Create(ctx context.Context, record *HostedOpenAIAccountRecord) error
	GetByID(ctx context.Context, id int64) (*HostedOpenAIAccountRecord, error)
	GetByIDForUser(ctx context.Context, id, userID int64) (*HostedOpenAIAccountRecord, error)
	ListByUser(ctx context.Context, userID int64) ([]HostedOpenAIAccountRecord, error)
	List(ctx context.Context, search, status string) ([]HostedOpenAIAccountRecord, error)
	UpdateConfig(ctx context.Context, id int64, proxyID *int64, model string, supportsMessages bool) error
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdateQuotaMeta(ctx context.Context, id int64, planType string, refreshedAt time.Time) error
	UpdateIdentity(ctx context.Context, id int64, email, planType string) error
	DeleteSubscription(ctx context.Context, subscriptionID int64) error
}

type HostedOpenAIQuotaWindow struct {
	UsedPercent   float64    `json:"used_percent"`
	ResetAt       *time.Time `json:"reset_at,omitempty"`
	WindowMinutes int        `json:"window_minutes,omitempty"`
}

type HostedOpenAIQuota struct {
	PlanType    string                   `json:"plan_type,omitempty"`
	AccountType string                   `json:"account_type"`
	FiveHour    *HostedOpenAIQuotaWindow `json:"five_hour,omitempty"`
	Weekly      *HostedOpenAIQuotaWindow `json:"weekly,omitempty"`
	FetchedAt   *time.Time               `json:"fetched_at,omitempty"`
	Error       string                   `json:"error,omitempty"`
}

type HostedOpenAIAccountView struct {
	HostedOpenAIAccountRecord
	AccountStatus string            `json:"account_status"`
	Quota         HostedOpenAIQuota `json:"quota"`
}

type HostedProxyOption struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type HostedOpenAIOptions struct {
	Proxies         []HostedProxyOption `json:"proxies"`
	SuggestedModels []string            `json:"suggested_models"`
}

type CreateHostedOpenAIAccountInput struct {
	SessionID        string
	Code             string
	State            string
	RedirectURI      string
	Model            string
	ProxyID          *int64
	SupportsMessages bool
}

type UpdateHostedOpenAIAccountInput struct {
	Model            string
	ProxyID          *int64
	SupportsMessages *bool
}

type ReauthorizeHostedOpenAIAccountInput struct {
	SessionID   string
	Code        string
	State       string
	RedirectURI string
}

type HostedOpenAIAccountService struct {
	repo         HostedOpenAIAccountRepository
	adminService AdminService
	oauthService *OpenAIOAuthService
	subscription *SubscriptionService
	userRepo     UserRepository
	groupRepo    GroupRepository
	proxyRepo    ProxyRepository
	quotaService *OpenAIQuotaService
}

func NewHostedOpenAIAccountService(
	repo HostedOpenAIAccountRepository,
	adminService AdminService,
	oauthService *OpenAIOAuthService,
	subscription *SubscriptionService,
	userRepo UserRepository,
	groupRepo GroupRepository,
	proxyRepo ProxyRepository,
	quotaService *OpenAIQuotaService,
) *HostedOpenAIAccountService {
	return &HostedOpenAIAccountService{
		repo:         repo,
		adminService: adminService,
		oauthService: oauthService,
		subscription: subscription,
		userRepo:     userRepo,
		groupRepo:    groupRepo,
		proxyRepo:    proxyRepo,
		quotaService: quotaService,
	}
}

func (s *HostedOpenAIAccountService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI string) (*OpenAIAuthURLResult, error) {
	if s.oauthService == nil {
		return nil, infraerrors.ServiceUnavailable("OPENAI_OAUTH_UNAVAILABLE", "OpenAI OAuth is unavailable")
	}
	if proxyID != nil {
		if _, err := s.validProxy(ctx, proxyID); err != nil {
			return nil, err
		}
	}
	return s.oauthService.GenerateAuthURL(ctx, proxyID, redirectURI, PlatformOpenAI)
}

func (s *HostedOpenAIAccountService) Options(ctx context.Context) (*HostedOpenAIOptions, error) {
	proxies, err := s.proxyRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active proxies: %w", err)
	}
	proxyOptions := make([]HostedProxyOption, 0, len(proxies))
	for _, proxy := range proxies {
		proxyOptions = append(proxyOptions, HostedProxyOption{ID: proxy.ID, Name: proxy.Name})
	}
	sort.Slice(proxyOptions, func(i, j int) bool { return proxyOptions[i].Name < proxyOptions[j].Name })

	groups, err := s.groupRepo.ListActiveByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return nil, fmt.Errorf("list OpenAI groups: %w", err)
	}
	modelSet := make(map[string]struct{})
	for _, group := range groups {
		for _, model := range group.ModelAllowlist.Models {
			if model = strings.TrimSpace(model); model != "" {
				modelSet[model] = struct{}{}
			}
		}
		for _, pricing := range group.ModelPricing {
			for _, model := range pricing.Models {
				if model = strings.TrimSpace(model); model != "" {
					modelSet[model] = struct{}{}
				}
			}
		}
	}
	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
	}
	sort.Strings(models)
	return &HostedOpenAIOptions{Proxies: proxyOptions, SuggestedModels: models}, nil
}

func (s *HostedOpenAIAccountService) ListForUser(ctx context.Context, userID int64) ([]HostedOpenAIAccountView, error) {
	records, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.views(ctx, records), nil
}

func (s *HostedOpenAIAccountService) ListForAdmin(ctx context.Context, search, status string) ([]HostedOpenAIAccountView, error) {
	records, err := s.repo.List(ctx, search, status)
	if err != nil {
		return nil, err
	}
	return s.views(ctx, records), nil
}

func (s *HostedOpenAIAccountService) Create(
	ctx context.Context,
	userID int64,
	input CreateHostedOpenAIAccountInput,
) (*HostedOpenAIAccountView, error) {
	model := strings.TrimSpace(input.Model)
	if model == "" {
		return nil, infraerrors.BadRequest("HOSTED_OPENAI_MODEL_REQUIRED", "please choose a model")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if input.ProxyID != nil {
		if _, err := s.validProxy(ctx, input.ProxyID); err != nil {
			return nil, err
		}
	}
	tokenInfo, err := s.oauthService.ExchangeCode(ctx, &OpenAIExchangeCodeInput{
		SessionID:   input.SessionID,
		Code:        input.Code,
		State:       input.State,
		RedirectURI: input.RedirectURI,
		ProxyID:     input.ProxyID,
	})
	if err != nil {
		return nil, err
	}
	credentials := s.oauthService.BuildAccountCredentials(tokenInfo)
	email := strings.TrimSpace(tokenInfo.Email)
	if email == "" {
		email = strings.TrimSpace(user.Email)
	}
	label := strings.TrimSpace(user.Username)
	if label == "" {
		label = email
	}
	groupName := truncateRunes(fmt.Sprintf("托管 · %s · %d", label, time.Now().Unix()%100000), 100)

	group, err := s.adminService.CreateGroup(ctx, &CreateGroupInput{
		Name:                  groupName,
		Description:           "用户自持 OpenAI OAuth 账号的专属托管分组",
		Platform:              PlatformOpenAI,
		RateMultiplier:        1,
		IsExclusive:           true,
		SubscriptionType:      SubscriptionTypeSubscription,
		AllowMessagesDispatch: input.SupportsMessages,
		RequireOAuthOnly:      true,
		DefaultMappedModel:    model,
		ModelAllowlist: GroupModelAllowlist{
			Enabled: true,
			Models:  []string{model},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create hosted group: %w", err)
	}

	account, err := s.adminService.CreateAccount(ctx, &CreateAccountInput{
		Name:        hostedFirstNonEmpty(email, label, "Hosted OpenAI Account"),
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: credentials,
		Extra: map[string]any{
			"hosted_owner_user_id": userID,
			"hosted_managed":       true,
		},
		ProxyID:               input.ProxyID,
		Concurrency:           3,
		Priority:              0,
		GroupIDs:              []int64{group.ID},
		SkipDefaultGroupBind:  true,
		SkipMixedChannelCheck: true,
	})
	if err != nil {
		_ = s.adminService.DeleteGroupIfEmpty(ctx, group.ID)
		return nil, fmt.Errorf("create hosted OAuth account: %w", err)
	}

	sub, err := s.subscription.AssignSubscription(ctx, &AssignSubscriptionInput{
		UserID:       userID,
		GroupID:      group.ID,
		ValidityDays: MaxValidityDays,
		AssignedBy:   userID,
		Notes:        "hosted OpenAI OAuth",
	})
	if err != nil {
		s.cleanupCreateFailure(ctx, account.ID, group.ID, 0)
		return nil, fmt.Errorf("assign hosted subscription: %w", err)
	}
	if err := s.userRepo.AddGroupToAllowedGroups(ctx, userID, group.ID); err != nil {
		s.cleanupCreateFailure(ctx, account.ID, group.ID, sub.ID)
		return nil, fmt.Errorf("grant hosted group access: %w", err)
	}

	record := &HostedOpenAIAccountRecord{
		UserID:           userID,
		AccountID:        account.ID,
		GroupID:          group.ID,
		SubscriptionID:   &sub.ID,
		ProxyID:          input.ProxyID,
		Model:            model,
		SupportsMessages: input.SupportsMessages,
		Status:           "active",
		AccountEmail:     email,
		PlanType:         hostedFirstNonEmpty(strings.TrimSpace(tokenInfo.PlanType), accountCredentialString(account, "plan_type")),
	}
	if err := s.repo.Create(ctx, record); err != nil {
		s.cleanupCreateFailure(ctx, account.ID, group.ID, sub.ID)
		return nil, err
	}
	record.UserName = hostedFirstNonEmpty(user.Username, user.Email)
	record.UserEmail = user.Email
	record.GroupName = group.Name
	if input.ProxyID != nil {
		if proxy, proxyErr := s.proxyRepo.GetByID(ctx, *input.ProxyID); proxyErr == nil && proxy != nil {
			record.ProxyName = proxy.Name
		}
	}
	view := HostedOpenAIAccountView{
		HostedOpenAIAccountRecord: *record,
		AccountStatus:             account.Status,
		Quota:                     buildHostedQuotaFromAccount(account),
	}
	return &view, nil
}

func (s *HostedOpenAIAccountService) UpdateForUser(
	ctx context.Context,
	userID, id int64,
	input UpdateHostedOpenAIAccountInput,
) (*HostedOpenAIAccountView, error) {
	record, err := s.repo.GetByIDForUser(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	return s.update(ctx, record, input)
}

func (s *HostedOpenAIAccountService) UpdateForAdmin(
	ctx context.Context,
	id int64,
	input UpdateHostedOpenAIAccountInput,
) (*HostedOpenAIAccountView, error) {
	record, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.update(ctx, record, input)
}

func (s *HostedOpenAIAccountService) update(
	ctx context.Context,
	record *HostedOpenAIAccountRecord,
	input UpdateHostedOpenAIAccountInput,
) (*HostedOpenAIAccountView, error) {
	if record.Status == "archived" {
		return nil, infraerrors.Conflict("HOSTED_OPENAI_ACCOUNT_ARCHIVED", "hosted account is archived")
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = record.Model
	}
	supportsMessages := record.SupportsMessages
	if input.SupportsMessages != nil {
		supportsMessages = *input.SupportsMessages
	}
	if input.ProxyID != nil {
		if _, err := s.validProxy(ctx, input.ProxyID); err != nil {
			return nil, err
		}
	}
	group, err := s.adminService.GetGroup(ctx, record.GroupID)
	if err != nil {
		return nil, fmt.Errorf("get hosted group: %w", err)
	}
	_, err = s.adminService.UpdateGroup(ctx, record.GroupID, &UpdateGroupInput{
		Platform:              group.Platform,
		AllowMessagesDispatch: &supportsMessages,
		DefaultMappedModel:    &model,
		ModelAllowlist: &GroupModelAllowlist{
			Enabled: true,
			Models:  []string{model},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("update hosted group: %w", err)
	}
	accountProxyID := int64(0)
	if input.ProxyID != nil {
		accountProxyID = *input.ProxyID
	}
	if _, err := s.adminService.UpdateAccount(ctx, record.AccountID, &UpdateAccountInput{ProxyID: &accountProxyID}); err != nil {
		return nil, fmt.Errorf("update hosted account proxy: %w", err)
	}
	if err := s.repo.UpdateConfig(ctx, record.ID, input.ProxyID, model, supportsMessages); err != nil {
		return nil, err
	}
	updated, err := s.repo.GetByID(ctx, record.ID)
	if err != nil {
		return nil, err
	}
	views := s.views(ctx, []HostedOpenAIAccountRecord{*updated})
	if len(views) == 0 {
		return nil, ErrHostedOpenAIAccountNotFound
	}
	return &views[0], nil
}

func (s *HostedOpenAIAccountService) RefreshQuotaForUser(ctx context.Context, userID, id int64) (*HostedOpenAIAccountView, error) {
	record, err := s.repo.GetByIDForUser(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	return s.refreshQuota(ctx, record)
}

func (s *HostedOpenAIAccountService) RefreshQuotaForAdmin(ctx context.Context, id int64) (*HostedOpenAIAccountView, error) {
	record, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.refreshQuota(ctx, record)
}

func (s *HostedOpenAIAccountService) refreshQuota(ctx context.Context, record *HostedOpenAIAccountRecord) (*HostedOpenAIAccountView, error) {
	if s.quotaService == nil {
		return nil, infraerrors.ServiceUnavailable("OPENAI_QUOTA_UNAVAILABLE", "OpenAI quota service is unavailable")
	}
	usage, err := s.quotaService.QueryUsage(ctx, record.AccountID)
	if err != nil {
		return nil, err
	}
	if usage == nil {
		return nil, infraerrors.New(502, "OPENAI_QUOTA_EMPTY", "OpenAI quota returned no data")
	}
	if err := s.quotaService.CachePostResetSnapshot(ctx, record.AccountID, usage); err != nil {
		slog.Warn("hosted_openai_quota_cache_failed", "account_id", record.AccountID, "error", err)
	}
	planType := hostedFirstNonEmpty(strings.TrimSpace(usage.PlanType), record.PlanType)
	if err := s.repo.UpdateQuotaMeta(ctx, record.ID, planType, time.Unix(usage.FetchedAt, 0).UTC()); err != nil {
		return nil, err
	}
	account, _ := s.adminService.GetAccount(ctx, record.AccountID)
	view := HostedOpenAIAccountView{
		HostedOpenAIAccountRecord: *record,
		AccountStatus:             accountStatus(account),
		Quota:                     buildHostedQuotaFromUsage(usage),
	}
	view.PlanType = planType
	refreshedAt := time.Unix(usage.FetchedAt, 0).UTC()
	view.LastQuotaRefreshedAt = &refreshedAt
	return &view, nil
}

func (s *HostedOpenAIAccountService) ReauthorizeForUser(
	ctx context.Context,
	userID, id int64,
	input ReauthorizeHostedOpenAIAccountInput,
) (*HostedOpenAIAccountView, error) {
	record, err := s.repo.GetByIDForUser(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if record.Status == "archived" {
		return nil, infraerrors.Conflict("HOSTED_OPENAI_ACCOUNT_ARCHIVED", "hosted account is archived")
	}
	tokenInfo, err := s.oauthService.ExchangeCode(ctx, &OpenAIExchangeCodeInput{
		SessionID:   input.SessionID,
		Code:        input.Code,
		State:       input.State,
		RedirectURI: input.RedirectURI,
		ProxyID:     record.ProxyID,
	})
	if err != nil {
		return nil, err
	}
	credentials := s.oauthService.BuildAccountCredentials(tokenInfo)
	if _, err := s.adminService.UpdateAccount(ctx, record.AccountID, &UpdateAccountInput{Credentials: credentials}); err != nil {
		return nil, fmt.Errorf("update hosted OAuth credentials: %w", err)
	}
	email := hostedFirstNonEmpty(strings.TrimSpace(tokenInfo.Email), record.AccountEmail)
	planType := hostedFirstNonEmpty(strings.TrimSpace(tokenInfo.PlanType), record.PlanType)
	if err := s.repo.UpdateIdentity(ctx, record.ID, email, planType); err != nil {
		return nil, err
	}
	updated, err := s.repo.GetByIDForUser(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	views := s.views(ctx, []HostedOpenAIAccountRecord{*updated})
	return &views[0], nil
}

func (s *HostedOpenAIAccountService) SetStatusForAdmin(ctx context.Context, id int64, status string) (*HostedOpenAIAccountView, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	if status != "active" && status != "suspended" && status != "archived" {
		return nil, infraerrors.BadRequest("HOSTED_OPENAI_ACCOUNT_STATUS_INVALID", "status must be active, suspended, or archived")
	}
	record, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if _, err := s.adminService.SetAccountSchedulable(ctx, record.AccountID, status == "active"); err != nil {
		return nil, fmt.Errorf("update hosted account scheduling state: %w", err)
	}
	if err := s.repo.UpdateStatus(ctx, id, status); err != nil {
		return nil, err
	}
	updated, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	views := s.views(ctx, []HostedOpenAIAccountRecord{*updated})
	return &views[0], nil
}

func (s *HostedOpenAIAccountService) views(ctx context.Context, records []HostedOpenAIAccountRecord) []HostedOpenAIAccountView {
	if len(records) == 0 {
		return []HostedOpenAIAccountView{}
	}
	accountIDs := make([]int64, 0, len(records))
	for _, record := range records {
		accountIDs = append(accountIDs, record.AccountID)
	}
	accounts, err := s.adminService.GetAccountsByIDs(ctx, accountIDs)
	if err != nil {
		slog.Warn("hosted_openai_accounts_batch_load_failed", "error", err)
	}
	accountByID := make(map[int64]*Account, len(accounts))
	for _, account := range accounts {
		if account != nil {
			accountByID[account.ID] = account
		}
	}
	views := make([]HostedOpenAIAccountView, 0, len(records))
	for _, record := range records {
		account := accountByID[record.AccountID]
		quota := HostedOpenAIQuota{AccountType: hostedAccountType(record.PlanType)}
		if account == nil {
			quota.Error = "账号不存在"
		} else {
			quota = buildHostedQuotaFromAccount(account)
		}
		views = append(views, HostedOpenAIAccountView{
			HostedOpenAIAccountRecord: record,
			AccountStatus:             accountStatus(account),
			Quota:                     quota,
		})
	}
	return views
}

func (s *HostedOpenAIAccountService) validProxy(ctx context.Context, proxyID *int64) (*Proxy, error) {
	if proxyID == nil || *proxyID <= 0 {
		return nil, nil
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil || proxy == nil {
		return nil, infraerrors.BadRequest("HOSTED_PROXY_INVALID", "selected proxy is unavailable")
	}
	if !proxy.IsActive() || proxy.IsExpired(time.Now()) {
		return nil, infraerrors.BadRequest("HOSTED_PROXY_INACTIVE", "selected proxy is inactive or expired")
	}
	return proxy, nil
}

func (s *HostedOpenAIAccountService) cleanupCreateFailure(ctx context.Context, accountID, groupID, subscriptionID int64) {
	if subscriptionID > 0 {
		if err := s.repo.DeleteSubscription(ctx, subscriptionID); err != nil {
			slog.Warn("hosted_subscription_cleanup_failed", "subscription_id", subscriptionID, "error", err)
		}
	}
	if accountID > 0 {
		if err := s.adminService.DeleteAccount(ctx, accountID); err != nil {
			slog.Warn("hosted_account_cleanup_failed", "account_id", accountID, "error", err)
		}
	}
	if groupID > 0 {
		if err := s.adminService.DeleteGroupIfEmpty(ctx, groupID); err != nil && !errors.Is(err, ErrGroupNotFound) {
			slog.Warn("hosted_group_cleanup_failed", "group_id", groupID, "error", err)
		}
	}
}

func buildHostedQuotaFromAccount(account *Account) HostedOpenAIQuota {
	planType := accountCredentialString(account, "plan_type")
	quota := HostedOpenAIQuota{
		PlanType:    planType,
		AccountType: hostedAccountType(planType),
	}
	if account == nil || account.Extra == nil {
		return quota
	}
	quota.FiveHour = hostedQuotaWindowFromExtra(account.Extra, "5h")
	quota.Weekly = hostedQuotaWindowFromExtra(account.Extra, "7d")
	if fetchedAt, ok := parseHostedQuotaTime(account.Extra["codex_usage_updated_at"]); ok {
		quota.FetchedAt = &fetchedAt
	}
	return quota
}

func buildHostedQuotaFromUsage(usage *OpenAIQuotaUsage) HostedOpenAIQuota {
	if usage == nil {
		return HostedOpenAIQuota{AccountType: "OpenAI OAuth"}
	}
	quota := HostedOpenAIQuota{
		PlanType:    usage.PlanType,
		AccountType: hostedAccountType(usage.PlanType),
	}
	if usage.FetchedAt > 0 {
		t := time.Unix(usage.FetchedAt, 0).UTC()
		quota.FetchedAt = &t
	}
	if usage.RateLimit != nil {
		windows := []*OpenAIRateLimitWindow{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow}
		for _, window := range windows {
			if window == nil {
				continue
			}
			converted := hostedQuotaWindowFromRateLimit(window, usage.FetchedAt)
			switch {
			case window.LimitWindowSeconds > 0 && window.LimitWindowSeconds >= 6*24*60*60:
				quota.Weekly = converted
			case window.LimitWindowSeconds > 0 && window.LimitWindowSeconds <= 6*60*60:
				quota.FiveHour = converted
			default:
				if quota.FiveHour == nil {
					quota.FiveHour = converted
				} else if quota.Weekly == nil {
					quota.Weekly = converted
				}
			}
		}
	}
	return quota
}

func hostedQuotaWindowFromRateLimit(window *OpenAIRateLimitWindow, fetchedAt int64) *HostedOpenAIQuotaWindow {
	if window == nil {
		return nil
	}
	result := &HostedOpenAIQuotaWindow{
		UsedPercent:   window.UsedPercent,
		WindowMinutes: int(window.LimitWindowSeconds / 60),
	}
	if window.ResetAt > 0 {
		t := time.Unix(window.ResetAt, 0).UTC()
		result.ResetAt = &t
	} else if fetchedAt > 0 && window.ResetAfterSeconds > 0 {
		t := time.Unix(fetchedAt+window.ResetAfterSeconds, 0).UTC()
		result.ResetAt = &t
	}
	return result
}

func hostedQuotaWindowFromExtra(extra map[string]any, suffix string) *HostedOpenAIQuotaWindow {
	used, ok := parseHostedQuotaFloat(extra["codex_"+suffix+"_used_percent"])
	if !ok {
		return nil
	}
	result := &HostedOpenAIQuotaWindow{UsedPercent: used}
	if minutes, ok := parseHostedQuotaInt(extra["codex_"+suffix+"_window_minutes"]); ok {
		result.WindowMinutes = int(minutes)
	}
	if resetAt, ok := parseHostedQuotaTime(extra["codex_"+suffix+"_reset_at"]); ok {
		result.ResetAt = &resetAt
	}
	return result
}

func parseHostedQuotaFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case jsonNumberProvider:
		number, err := typed.Float64()
		return number, err == nil
	default:
		return 0, false
	}
}

type jsonNumberProvider interface {
	Float64() (float64, error)
}

func parseHostedQuotaInt(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case jsonInt64Provider:
		number, err := typed.Int64()
		return number, err == nil
	default:
		return 0, false
	}
}

type jsonInt64Provider interface {
	Int64() (int64, error)
}

func parseHostedQuotaTime(value any) (time.Time, bool) {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

func hostedAccountType(planType string) string {
	switch strings.ToLower(strings.TrimSpace(planType)) {
	case "plus":
		return "ChatGPT Plus"
	case "pro":
		return "ChatGPT Pro"
	case "team":
		return "ChatGPT Team"
	case "business":
		return "ChatGPT Business"
	case "enterprise":
		return "ChatGPT Enterprise"
	case "free":
		return "ChatGPT Free"
	default:
		return "OpenAI OAuth"
	}
}

func accountStatus(account *Account) string {
	if account == nil {
		return "missing"
	}
	return account.Status
}

func accountCredentialString(account *Account, key string) string {
	if account == nil {
		return ""
	}
	value, _ := account.Credentials[key].(string)
	return strings.TrimSpace(value)
}

func hostedFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
