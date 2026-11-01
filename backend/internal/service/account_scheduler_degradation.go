package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	AccountDegradationStatusPass     = "pass"
	AccountDegradationStatusDegraded = "degraded"
	AccountDegradationStatusError    = "error"

	accountSchedulerDefaultIntervalSeconds = 600
	accountSchedulerMinimumIntervalSeconds = 60
	accountSchedulerMaximumIntervalSeconds = 86400
	accountSchedulerHistoryWindow          = 24 * time.Hour
	accountSchedulerProbeWorkers           = 4
	accountSchedulerDegradationWeight      = 1.5
	accountSchedulerDefaultTestModel       = "gpt-6-astra"
	AccountDegradationReuseWindow          = 30 * time.Minute
	DegradationSourceAccountAggregate      = "account_aggregate"
	DegradationSourceChannelFallback       = "channel_fallback"
)

var (
	ErrAccountSchedulerInvalidSetting = errors.New("invalid account scheduler setting")
	ErrAccountSchedulerOpenAIOnly     = errors.New("degradation probes only support OpenAI groups")
)

type AccountSchedulerGroup struct {
	ID              int64
	Name            string
	Platform        string
	Status          string
	ModelCandidates []string
}

type AccountSchedulerGroupSetting struct {
	GroupID                  int64
	DegradationEnabled       bool
	DegradationFactorEnabled bool
	IntervalSeconds          int
	TestModel                string
	TestReasoningEffort      string
	UpdatedAt                time.Time
}

type AccountSchedulerAccount struct {
	ID                      int64
	GroupID                 int64
	Name                    string
	Platform                string
	Type                    string
	Status                  string
	Schedulable             bool
	Priority                int
	GroupPriority           int
	Concurrency             int
	RateMultiplier          *float64
	RateLimitResetAt        *time.Time
	OverloadUntil           *time.Time
	TempUnschedulableUntil  *time.Time
	TempUnschedulableReason string
	ExpiresAt               *time.Time
	AutoPauseOnExpired      bool
	DispatchCount           int64
}

type AccountDegradationStat struct {
	GroupID          int64
	AccountID        int64
	PassCount        int64
	DegradedCount    int64
	ErrorCount       int64
	LatestStatus     string
	LatestCheckedAt  *time.Time
	AverageLatencyMs float64
}

type GroupAccountDegradationSummary struct {
	GroupID         int64
	SampleCount     int64
	PassCount       int64
	DegradedCount   int64
	ErrorCount      int64
	LatestCheckedAt *time.Time
	LatestModel     string
}

func (s GroupAccountDegradationSummary) IsFresh(now time.Time) bool {
	return s.LatestCheckedAt != nil && now.Sub(*s.LatestCheckedAt) <= AccountDegradationReuseWindow
}

func (s GroupAccountDegradationSummary) PassRate() float64 {
	if s.SampleCount <= 0 {
		return 0
	}
	return float64(s.PassCount) / float64(s.SampleCount)
}

func (s AccountDegradationStat) SampleCount() int64 {
	return s.PassCount + s.DegradedCount
}

func (s AccountDegradationStat) DegradationRate() (float64, bool) {
	samples := s.SampleCount()
	if samples <= 0 {
		return 0, false
	}
	return float64(s.DegradedCount) / float64(samples), true
}

type AccountDegradationProbe struct {
	ID              int64     `json:"id"`
	AccountID       int64     `json:"account_id"`
	AccountName     string    `json:"account_name"`
	GroupID         int64     `json:"group_id"`
	Model           string    `json:"model"`
	ReasoningEffort string    `json:"reasoning_effort"`
	Status          string    `json:"status"`
	ResponseText    string    `json:"response_text"`
	ErrorMessage    string    `json:"error_message"`
	LatencyMs       int64     `json:"latency_ms"`
	CheckedAt       time.Time `json:"checked_at"`
}

type AccountSchedulerOverview struct {
	ScoringMode       string                      `json:"scoring_mode"`
	OAuthFirst        bool                        `json:"oauth_first"`
	PriorityIgnored   bool                        `json:"priority_ignored"`
	DegradationWeight float64                     `json:"degradation_weight"`
	AdvancedEnabled   bool                        `json:"advanced_enabled"`
	TestedGroups      int                         `json:"tested_groups"`
	EnabledGroups     int                         `json:"enabled_groups"`
	TotalAccounts     int                         `json:"total_accounts"`
	AvailableAccounts int                         `json:"available_accounts"`
	Groups            []AccountSchedulerGroupView `json:"groups"`
	RecentProbes      []AccountDegradationProbe   `json:"recent_probes"`
	GeneratedAt       time.Time                   `json:"generated_at"`
}

type AccountSchedulerGroupView struct {
	ID                       int64                 `json:"id"`
	Name                     string                `json:"name"`
	Platform                 string                `json:"platform"`
	DegradationSupported     bool                  `json:"degradation_supported"`
	Status                   string                `json:"status"`
	DegradationEnabled       bool                  `json:"degradation_enabled"`
	DegradationFactorEnabled bool                  `json:"degradation_factor_enabled"`
	IntervalSeconds          int                   `json:"interval_seconds"`
	TestModel                string                `json:"test_model"`
	TestReasoningEffort      string                `json:"test_reasoning_effort"`
	ScoreMax                 float64               `json:"score_max"`
	ModelCandidates          []string              `json:"model_candidates"`
	OAuthFirst               bool                  `json:"oauth_first"`
	LowestAccountID          *int64                `json:"lowest_account_id,omitempty"`
	Accounts                 []AccountSchedulerRow `json:"accounts"`
}

type AccountSchedulerRow struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Platform           string     `json:"platform"`
	CredentialType     string     `json:"credential_type"`
	OAuth              bool       `json:"oauth"`
	Available          bool       `json:"available"`
	AvailabilityReason string     `json:"availability_reason"`
	DispatchCount      int64      `json:"dispatch_count"`
	ActiveDispatches   int        `json:"active_dispatches"`
	DegradationRate    *float64   `json:"degradation_rate,omitempty"`
	SampleCount        int64      `json:"sample_count"`
	PassCount          int64      `json:"pass_count"`
	DegradedCount      int64      `json:"degraded_count"`
	ErrorCount         int64      `json:"error_count"`
	LatestStatus       string     `json:"latest_status"`
	LatestCheckedAt    *time.Time `json:"latest_checked_at,omitempty"`
	AverageLatencyMs   float64    `json:"average_latency_ms"`
	Rank               int        `json:"rank"`
	Score              float64    `json:"score"`
	ScoreWeight        float64    `json:"score_weight"`
	DegradationFactor  float64    `json:"degradation_factor"`
	LowestDegradation  bool       `json:"lowest_degradation"`
}

type AccountSchedulerSettingInput struct {
	GroupID                  int64    `json:"group_id"`
	DegradationEnabled       *bool    `json:"degradation_enabled"`
	DegradationFactorEnabled *bool    `json:"degradation_factor_enabled"`
	IntervalSeconds          *int     `json:"interval_seconds"`
	TestModel                *string  `json:"test_model"`
	TestReasoningEffort      *string  `json:"test_reasoning_effort"`
	DegradationWeight        *float64 `json:"degradation_weight"`
}

type AccountSchedulerRunInput struct {
	GroupID         int64  `json:"group_id"`
	AccountID       int64  `json:"account_id,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type AccountSchedulerRunResult struct {
	Run     int                       `json:"run"`
	Skipped int                       `json:"skipped"`
	Failed  int                       `json:"failed"`
	Results []AccountDegradationProbe `json:"results"`
	Errors  []string                  `json:"errors,omitempty"`
}

type AccountDegradationScore struct {
	Rate       float64
	HasSamples bool
	Samples    int64
}

type AccountDegradationScoreProvider interface {
	AccountDegradationScores(ctx context.Context, groupID int64, accountIDs []int64) map[int64]AccountDegradationScore
	DegradationFactorEnabled(ctx context.Context, groupID int64) bool
}

type AccountSchedulerRepository interface {
	ListGroups(ctx context.Context) ([]AccountSchedulerGroup, error)
	ListGroupSettings(ctx context.Context) (map[int64]AccountSchedulerGroupSetting, error)
	UpsertGroupSetting(ctx context.Context, setting AccountSchedulerGroupSetting) (*AccountSchedulerGroupSetting, error)
	ListGroupAccounts(ctx context.Context, groupIDs []int64) ([]AccountSchedulerAccount, error)
	ListDegradationStats(ctx context.Context, since time.Time) ([]AccountDegradationStat, error)
	ListRecentProbes(ctx context.Context, groupID int64, limit int) ([]AccountDegradationProbe, error)
	InsertProbe(ctx context.Context, probe *AccountDegradationProbe) error
}

type accountSchedulerProbeProfileReader interface {
	ListGroupPrimaryModels(ctx context.Context, groupIDs []int64) (map[int64]string, error)
}

type AccountSchedulerService struct {
	repo           AccountSchedulerRepository
	accountTestSvc *AccountTestService
	gateway        *OpenAIGatewayService
	settingRepo    SettingRepository

	cacheMu sync.RWMutex
	cache   map[int64]accountDegradationScoreCacheEntry

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	doneCh    chan struct{}
	runMu     sync.Mutex
}

type accountDegradationScoreCacheEntry struct {
	expiresAt     time.Time
	scores        map[int64]AccountDegradationScore
	factorEnabled bool
}

func NewAccountSchedulerService(
	repo AccountSchedulerRepository,
	accountTestSvc *AccountTestService,
	gateway *OpenAIGatewayService,
	settingRepo SettingRepository,
) *AccountSchedulerService {
	return &AccountSchedulerService{
		repo:           repo,
		accountTestSvc: accountTestSvc,
		gateway:        gateway,
		settingRepo:    settingRepo,
		cache:          make(map[int64]accountDegradationScoreCacheEntry),
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
	}
}

func (s *AccountSchedulerService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		if s.gateway != nil {
			s.gateway.SetAccountDegradationScoreProvider(s)
		}
		go s.runLoop()
	})
}

func (s *AccountSchedulerService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
		select {
		case <-s.doneCh:
		case <-time.After(5 * time.Second):
		}
	})
}

func (s *AccountSchedulerService) runLoop() {
	defer close(s.doneCh)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			_, err := s.RunScheduled(ctx)
			cancel()
			if err != nil {
				logger.LegacyPrintf("service.account_scheduler", "[AccountScheduler] scheduled probe failed: %v", err)
			}
		}
	}
}

func (s *AccountSchedulerService) Overview(ctx context.Context) (*AccountSchedulerOverview, error) {
	groups, err := s.repo.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.repo.ListGroupSettings(ctx)
	if err != nil {
		return nil, err
	}
	groupIDs := make([]int64, 0, len(groups))
	for _, group := range groups {
		groupIDs = append(groupIDs, group.ID)
	}
	accounts, err := s.repo.ListGroupAccounts(ctx, groupIDs)
	if err != nil {
		return nil, err
	}
	activeDispatches := s.activeAccountDispatches(ctx, accounts)
	stats, err := s.repo.ListDegradationStats(ctx, time.Now().Add(-accountSchedulerHistoryWindow))
	if err != nil {
		return nil, err
	}
	recent, err := s.repo.ListRecentProbes(ctx, 0, 80)
	if err != nil {
		return nil, err
	}

	accountsByGroup := make(map[int64][]AccountSchedulerAccount)
	for _, account := range accounts {
		accountsByGroup[account.GroupID] = append(accountsByGroup[account.GroupID], account)
	}
	statsByKey := make(map[string]AccountDegradationStat, len(stats))
	for _, stat := range stats {
		statsByKey[accountSchedulerStatKey(stat.GroupID, stat.AccountID)] = stat
	}

	weight, _ := s.degradationWeight(ctx)
	advancedEnabled, _ := s.advancedSchedulerEnabled(ctx)
	probeModels := s.accountSchedulerGroupPrimaryModels(ctx, groups)
	scoreMax := 0.0
	if s.gateway != nil {
		scoreMax = s.gateway.OpenAIAccountSchedulerScoreMaximum(ctx)
	}
	overview := &AccountSchedulerOverview{
		ScoringMode:       "score",
		OAuthFirst:        true,
		PriorityIgnored:   true,
		DegradationWeight: weight,
		AdvancedEnabled:   advancedEnabled,
		GeneratedAt:       time.Now(),
	}

	for _, group := range groups {
		groupAccounts := accountsByGroup[group.ID]
		degradationSupported := accountSchedulerGroupSupportsDegradation(group)
		setting := settings[group.ID]
		if setting.GroupID == 0 {
			setting = AccountSchedulerGroupSetting{
				GroupID:                  group.ID,
				DegradationFactorEnabled: false,
				IntervalSeconds:          accountSchedulerDefaultIntervalSeconds,
				TestModel:                accountSchedulerDefaultTestModel,
				TestReasoningEffort:      "medium",
			}
		}
		if setting.IntervalSeconds <= 0 {
			setting.IntervalSeconds = accountSchedulerDefaultIntervalSeconds
		}
		if degradationSupported && strings.TrimSpace(setting.TestModel) == "" {
			setting.TestModel = resolveAccountSchedulerProbeModel(setting, group, probeModels[group.ID])
		}
		setting.TestReasoningEffort = normalizeAccountSchedulerReasoningEffort(
			setting.TestModel,
			setting.TestReasoningEffort,
		)
		if !degradationSupported {
			setting.DegradationEnabled = false
			setting.TestModel = ""
		}

		rows := make([]AccountSchedulerRow, 0, len(groupAccounts))
		oauthFirst := false
		availableCount := 0
		groupHasStats := false
		for _, account := range groupAccounts {
			stat := AccountDegradationStat{}
			if degradationSupported {
				stat = statsByKey[accountSchedulerStatKey(group.ID, account.ID)]
				if stat.SampleCount() > 0 || stat.ErrorCount > 0 {
					groupHasStats = true
				}
			}
			available, reason := accountSchedulerAccountAvailability(account)
			if available {
				availableCount++
			}
			isOAuth := account.Type == AccountTypeOAuth || account.Type == AccountTypeSetupToken
			if isOAuth {
				oauthFirst = true
			}
			row := AccountSchedulerRow{
				ID:                 account.ID,
				Name:               account.Name,
				Platform:           account.Platform,
				CredentialType:     account.Type,
				OAuth:              isOAuth,
				Available:          available,
				AvailabilityReason: reason,
				DispatchCount:      account.DispatchCount,
				ActiveDispatches:   activeDispatches[account.ID],
				SampleCount:        stat.SampleCount(),
				PassCount:          stat.PassCount,
				DegradedCount:      stat.DegradedCount,
				ErrorCount:         stat.ErrorCount,
				LatestStatus:       stat.LatestStatus,
				LatestCheckedAt:    stat.LatestCheckedAt,
				AverageLatencyMs:   stat.AverageLatencyMs,
			}
			if degradationSupported {
				if setting.DegradationFactorEnabled {
					row.ScoreWeight = weight
				}
				if rate, ok := stat.DegradationRate(); ok {
					row.DegradationRate = &rate
					row.DegradationFactor = 1 - rate
				} else {
					row.DegradationFactor = 0.5
				}
			}
			rows = append(rows, row)
		}

		if degradationSupported {
			s.applyScores(ctx, group.ID, groupAccounts, rows)
			if setting.DegradationFactorEnabled {
				markLowestAccountDegradation(rows)
			}
		} else {
			rankAccountSchedulerRowsByAvailability(rows)
		}
		groupScoreMax := scoreMax
		if degradationSupported && !setting.DegradationFactorEnabled && groupScoreMax >= weight {
			groupScoreMax -= weight
		}
		groupView := AccountSchedulerGroupView{
			ID:                       group.ID,
			Name:                     group.Name,
			Platform:                 group.Platform,
			DegradationSupported:     degradationSupported,
			Status:                   group.Status,
			DegradationEnabled:       setting.DegradationEnabled,
			DegradationFactorEnabled: setting.DegradationFactorEnabled,
			IntervalSeconds:          setting.IntervalSeconds,
			TestModel:                setting.TestModel,
			TestReasoningEffort:      setting.TestReasoningEffort,
			ScoreMax:                 groupScoreMax,
			ModelCandidates:          group.ModelCandidates,
			OAuthFirst:               oauthFirst,
			Accounts:                 rows,
		}
		for i := range rows {
			if rows[i].LowestDegradation {
				id := rows[i].ID
				groupView.LowestAccountID = &id
				break
			}
		}
		overview.Groups = append(overview.Groups, groupView)
		overview.TotalAccounts += len(rows)
		overview.AvailableAccounts += availableCount
		if degradationSupported && setting.DegradationEnabled {
			overview.EnabledGroups++
		}
		if degradationSupported && groupHasStats {
			overview.TestedGroups++
		}
	}
	overview.RecentProbes = recent
	return overview, nil
}

func (s *AccountSchedulerService) UpdateSetting(ctx context.Context, input AccountSchedulerSettingInput) (*AccountSchedulerGroupSetting, error) {
	if input.GroupID <= 0 {
		return nil, ErrAccountSchedulerInvalidSetting
	}
	groups, err := s.repo.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	group := findAccountSchedulerGroup(groups, input.GroupID)
	if group == nil {
		return nil, ErrAccountSchedulerInvalidSetting
	}
	if !accountSchedulerGroupSupportsDegradation(*group) {
		return nil, ErrAccountSchedulerOpenAIOnly
	}
	settings, err := s.repo.ListGroupSettings(ctx)
	if err != nil {
		return nil, err
	}
	setting := settings[input.GroupID]
	if setting.GroupID == 0 {
		setting = AccountSchedulerGroupSetting{
			GroupID:                  input.GroupID,
			DegradationFactorEnabled: false,
			IntervalSeconds:          accountSchedulerDefaultIntervalSeconds,
			TestModel:                accountSchedulerDefaultTestModel,
			TestReasoningEffort:      "medium",
		}
	}
	if input.DegradationEnabled != nil {
		setting.DegradationEnabled = *input.DegradationEnabled
	}
	if input.DegradationFactorEnabled != nil {
		setting.DegradationFactorEnabled = *input.DegradationFactorEnabled
	}
	if input.IntervalSeconds != nil {
		setting.IntervalSeconds = *input.IntervalSeconds
	}
	if input.TestModel != nil {
		setting.TestModel = strings.TrimSpace(*input.TestModel)
	}
	if input.TestReasoningEffort != nil {
		rawEffort := strings.TrimSpace(*input.TestReasoningEffort)
		if rawEffort != "" && normalizeOpenAIReasoningEffortForModel(rawEffort, setting.TestModel) == "" {
			return nil, ErrAccountSchedulerInvalidSetting
		}
		setting.TestReasoningEffort = normalizeAccountSchedulerReasoningEffort(setting.TestModel, rawEffort)
	}
	if setting.IntervalSeconds < accountSchedulerMinimumIntervalSeconds || setting.IntervalSeconds > accountSchedulerMaximumIntervalSeconds {
		return nil, ErrAccountSchedulerInvalidSetting
	}
	updated, err := s.repo.UpsertGroupSetting(ctx, setting)
	if err != nil {
		return nil, err
	}
	s.invalidateCache(setting.GroupID)
	return updated, nil
}

func (s *AccountSchedulerService) RunNow(ctx context.Context, input AccountSchedulerRunInput) (*AccountSchedulerRunResult, error) {
	if input.GroupID <= 0 && input.AccountID <= 0 {
		return nil, ErrAccountSchedulerInvalidSetting
	}
	groups, err := s.repo.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.repo.ListGroupSettings(ctx)
	if err != nil {
		return nil, err
	}
	groupIDs := make([]int64, 0, len(groups))
	if input.GroupID > 0 {
		group := findAccountSchedulerGroup(groups, input.GroupID)
		if group == nil {
			return nil, ErrAccountSchedulerInvalidSetting
		}
		if !accountSchedulerGroupSupportsDegradation(*group) {
			return nil, ErrAccountSchedulerOpenAIOnly
		}
		groupIDs = append(groupIDs, input.GroupID)
	} else {
		for _, group := range groups {
			groupIDs = append(groupIDs, group.ID)
		}
	}
	accounts, err := s.repo.ListGroupAccounts(ctx, groupIDs)
	if err != nil {
		return nil, err
	}
	result := &AccountSchedulerRunResult{}
	probeModels := s.accountSchedulerGroupPrimaryModels(ctx, groups)
	for _, account := range accounts {
		if input.AccountID > 0 && account.ID != input.AccountID {
			continue
		}
		group := findAccountSchedulerGroup(groups, account.GroupID)
		if group == nil || !accountSchedulerGroupSupportsDegradation(*group) {
			if input.AccountID > 0 {
				return nil, ErrAccountSchedulerOpenAIOnly
			}
			result.Skipped++
			continue
		}
		setting := settings[account.GroupID]
		model := resolveAccountSchedulerProbeModel(setting, *group, probeModels[account.GroupID])
		if strings.TrimSpace(input.Model) != "" {
			model = strings.TrimSpace(input.Model)
		}
		reasoningEffort := normalizeAccountSchedulerReasoningEffort(model, setting.TestReasoningEffort)
		if strings.TrimSpace(input.ReasoningEffort) != "" {
			reasoningEffort = normalizeAccountSchedulerReasoningEffort(model, input.ReasoningEffort)
		}
		probe, err := s.runProbe(ctx, account, model, reasoningEffort)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s (#%d): %s", account.Name, account.ID, err.Error()))
			continue
		}
		result.Run++
		result.Results = append(result.Results, *probe)
		if probe.Status == AccountDegradationStatusError {
			result.Failed++
			if strings.TrimSpace(probe.ErrorMessage) != "" {
				result.Errors = append(result.Errors, fmt.Sprintf("%s (#%d): %s", account.Name, account.ID, probe.ErrorMessage))
			}
		}
	}
	s.invalidateCache(input.GroupID)
	return result, nil
}

func (s *AccountSchedulerService) RunScheduled(ctx context.Context) (*AccountSchedulerRunResult, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	groups, err := s.repo.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.repo.ListGroupSettings(ctx)
	if err != nil {
		return nil, err
	}
	groupIDs := make([]int64, 0)
	for groupID, setting := range settings {
		group := findAccountSchedulerGroup(groups, groupID)
		if setting.DegradationEnabled && group != nil && accountSchedulerGroupSupportsDegradation(*group) {
			groupIDs = append(groupIDs, groupID)
		}
	}
	if len(groupIDs) == 0 {
		return &AccountSchedulerRunResult{}, nil
	}
	accounts, err := s.repo.ListGroupAccounts(ctx, groupIDs)
	if err != nil {
		return nil, err
	}
	stats, err := s.repo.ListDegradationStats(ctx, time.Now().Add(-accountSchedulerHistoryWindow))
	if err != nil {
		return nil, err
	}
	latestByKey := make(map[string]*time.Time, len(stats))
	for _, stat := range stats {
		latestByKey[accountSchedulerStatKey(stat.GroupID, stat.AccountID)] = stat.LatestCheckedAt
	}

	result := &AccountSchedulerRunResult{}
	probeModels := s.accountSchedulerGroupPrimaryModels(ctx, groups)
	var due []AccountSchedulerAccount
	now := time.Now()
	for _, account := range accounts {
		setting := settings[account.GroupID]
		if !setting.DegradationEnabled {
			continue
		}
		latest := latestByKey[accountSchedulerStatKey(account.GroupID, account.ID)]
		if latest != nil && now.Sub(*latest) < time.Duration(setting.IntervalSeconds)*time.Second {
			result.Skipped++
			continue
		}
		due = append(due, account)
	}
	if len(due) == 0 {
		return result, nil
	}

	work := make(chan AccountSchedulerAccount)
	var mu sync.Mutex
	var wg sync.WaitGroup
	runOne := func(account AccountSchedulerAccount) {
		defer wg.Done()
		setting := settings[account.GroupID]
		group := findAccountSchedulerGroup(groups, account.GroupID)
		model := resolveAccountSchedulerProbeModel(setting, *group, probeModels[account.GroupID])
		reasoningEffort := normalizeAccountSchedulerReasoningEffort(model, setting.TestReasoningEffort)
		probe, err := s.runProbe(ctx, account, model, reasoningEffort)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s (#%d): %s", account.Name, account.ID, err.Error()))
			return
		}
		result.Run++
		result.Results = append(result.Results, *probe)
		if probe.Status == AccountDegradationStatusError {
			result.Failed++
			if strings.TrimSpace(probe.ErrorMessage) != "" {
				result.Errors = append(result.Errors, fmt.Sprintf("%s (#%d): %s", account.Name, account.ID, probe.ErrorMessage))
			}
		}
	}
	for i := 0; i < accountSchedulerProbeWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for account := range work {
				runOne(account)
			}
		}()
	}
	for _, account := range due {
		work <- account
	}
	close(work)
	wg.Wait()
	for groupID := range settings {
		s.invalidateCache(groupID)
	}
	return result, nil
}

func (s *AccountSchedulerService) AccountDegradationScores(ctx context.Context, groupID int64, accountIDs []int64) map[int64]AccountDegradationScore {
	if s == nil || s.repo == nil || groupID <= 0 || len(accountIDs) == 0 {
		return nil
	}
	entry, ok := s.accountDegradationScoreEntry(ctx, groupID)
	if !ok {
		return nil
	}
	return filterAccountDegradationScores(entry.scores, accountIDs)
}

func (s *AccountSchedulerService) DegradationFactorEnabled(ctx context.Context, groupID int64) bool {
	if s == nil || s.repo == nil || groupID <= 0 {
		return true
	}
	entry, ok := s.accountDegradationScoreEntry(ctx, groupID)
	if !ok {
		return true
	}
	return entry.factorEnabled
}

func (s *AccountSchedulerService) accountDegradationScoreEntry(
	ctx context.Context,
	groupID int64,
) (accountDegradationScoreCacheEntry, bool) {
	now := time.Now()
	s.cacheMu.RLock()
	entry, ok := s.cache[groupID]
	s.cacheMu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return entry, true
	}

	stats, err := s.repo.ListDegradationStats(ctx, now.Add(-accountSchedulerHistoryWindow))
	if err != nil {
		logger.LegacyPrintf("service.account_scheduler", "[AccountScheduler] load degradation scores failed: %v", err)
	}
	scores := make(map[int64]AccountDegradationScore)
	for _, stat := range stats {
		if stat.GroupID != groupID {
			continue
		}
		rate, hasSamples := stat.DegradationRate()
		scores[stat.AccountID] = AccountDegradationScore{
			Rate:       rate,
			HasSamples: hasSamples,
			Samples:    stat.SampleCount(),
		}
	}

	factorEnabled := true
	settings, settingsErr := s.repo.ListGroupSettings(ctx)
	if settingsErr != nil {
		logger.LegacyPrintf("service.account_scheduler", "[AccountScheduler] load group degradation setting failed: %v", settingsErr)
	} else if setting, exists := settings[groupID]; exists {
		factorEnabled = setting.DegradationFactorEnabled
	}
	entry = accountDegradationScoreCacheEntry{
		expiresAt:     now.Add(30 * time.Second),
		scores:        scores,
		factorEnabled: factorEnabled,
	}
	s.cacheMu.Lock()
	s.cache[groupID] = entry
	s.cacheMu.Unlock()
	return entry, true
}

func (s *AccountSchedulerService) runProbe(
	ctx context.Context,
	account AccountSchedulerAccount,
	model string,
	reasoningEffort string,
) (*AccountDegradationProbe, error) {
	startedAt := time.Now()
	probe := &AccountDegradationProbe{
		AccountID:       account.ID,
		AccountName:     account.Name,
		GroupID:         account.GroupID,
		Model:           model,
		ReasoningEffort: reasoningEffort,
		Status:          AccountDegradationStatusError,
		CheckedAt:       startedAt,
	}
	result, err := s.accountTestSvc.RunDegradationProbe(
		ctx,
		account.ID,
		model,
		monitorIntelligencePrompt,
		reasoningEffort,
	)
	probe.LatencyMs = time.Since(startedAt).Milliseconds()
	if err != nil || result == nil || result.Status != "success" {
		if err != nil {
			probe.ErrorMessage = err.Error()
		} else if result != nil {
			probe.ErrorMessage = result.ErrorMessage
		}
		if probe.ErrorMessage == "" {
			probe.ErrorMessage = "degradation probe failed"
		}
	} else {
		probe.ResponseText = strings.TrimSpace(result.ResponseText)
		if validateIntelligenceResponse(probe.ResponseText) {
			probe.Status = AccountDegradationStatusPass
		} else {
			probe.Status = AccountDegradationStatusDegraded
		}
	}
	if probe.LatencyMs <= 0 {
		probe.LatencyMs = 1
	}
	if err := s.repo.InsertProbe(ctx, probe); err != nil {
		return nil, err
	}
	return probe, nil
}

func (s *AccountSchedulerService) applyScores(
	ctx context.Context,
	groupID int64,
	accounts []AccountSchedulerAccount,
	rows []AccountSchedulerRow,
) {
	if s.gateway == nil || len(rows) == 0 {
		return
	}
	serviceAccounts := make([]*Account, 0, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		serviceAccounts = append(serviceAccounts, &Account{
			ID:                      account.ID,
			Name:                    account.Name,
			Platform:                account.Platform,
			Type:                    account.Type,
			Status:                  account.Status,
			Schedulable:             account.Schedulable,
			Priority:                account.Priority,
			Concurrency:             account.Concurrency,
			RateMultiplier:          account.RateMultiplier,
			RateLimitResetAt:        account.RateLimitResetAt,
			OverloadUntil:           account.OverloadUntil,
			TempUnschedulableUntil:  account.TempUnschedulableUntil,
			TempUnschedulableReason: account.TempUnschedulableReason,
			ExpiresAt:               account.ExpiresAt,
			AutoPauseOnExpired:      account.AutoPauseOnExpired,
		})
	}
	scores := s.gateway.BuildOpenAIAccountSchedulerScoreSnapshots(ctx, groupID, serviceAccounts, nil)
	for i := range rows {
		if score, ok := scores[rows[i].ID]; ok {
			rows[i].Score = score.BaseScore
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		if rows[i].Available != rows[j].Available {
			return rows[i].Available
		}
		return rows[i].ID < rows[j].ID
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
}

func (s *AccountSchedulerService) degradationWeight(context.Context) (float64, error) {
	return accountSchedulerDegradationWeight, nil
}

func (s *AccountSchedulerService) advancedSchedulerEnabled(ctx context.Context) (bool, error) {
	if s.settingRepo == nil {
		return false, nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{openAIAdvancedSchedulerSettingKey})
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(values[openAIAdvancedSchedulerSettingKey]), "true"), nil
}

func (s *AccountSchedulerService) invalidateCache(groupID int64) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if groupID > 0 {
		delete(s.cache, groupID)
		return
	}
	clear(s.cache)
}

func (s *AccountSchedulerService) activeAccountDispatches(
	ctx context.Context,
	accounts []AccountSchedulerAccount,
) map[int64]int {
	if s == nil || s.gateway == nil || s.gateway.concurrencyService == nil || len(accounts) == 0 {
		return nil
	}
	accountIDs := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		accountIDs = append(accountIDs, account.ID)
	}
	counts, err := s.gateway.concurrencyService.GetAccountConcurrencyBatch(ctx, accountIDs)
	if err != nil {
		logger.LegacyPrintf("service.account_scheduler", "[AccountScheduler] load active dispatches failed: %v", err)
		return nil
	}
	return counts
}

func accountSchedulerAccountAvailability(account AccountSchedulerAccount) (bool, string) {
	now := time.Now()
	switch {
	case account.Status != StatusActive:
		return false, "账号状态不可用"
	case !account.Schedulable:
		return false, "已关闭调度"
	case account.AutoPauseOnExpired && account.ExpiresAt != nil && !now.Before(*account.ExpiresAt):
		return false, "已过期"
	case account.OverloadUntil != nil && now.Before(*account.OverloadUntil):
		return false, "过载冷却"
	case account.RateLimitResetAt != nil && now.Before(*account.RateLimitResetAt):
		return false, "限流冷却"
	case account.TempUnschedulableUntil != nil && now.Before(*account.TempUnschedulableUntil):
		if strings.TrimSpace(account.TempUnschedulableReason) != "" {
			return false, account.TempUnschedulableReason
		}
		return false, "临时停调"
	default:
		return true, ""
	}
}

func markLowestAccountDegradation(rows []AccountSchedulerRow) {
	if len(rows) == 0 {
		return
	}
	lowest := -1
	var lowestRate float64
	for i := range rows {
		if rows[i].DegradationRate == nil || rows[i].SampleCount <= 0 {
			continue
		}
		rate := *rows[i].DegradationRate
		if lowest < 0 || rate < lowestRate || (rate == lowestRate && rows[i].Score > rows[lowest].Score) {
			lowest = i
			lowestRate = rate
		}
	}
	if lowest >= 0 {
		rows[lowest].LowestDegradation = true
	}
}

func filterAccountDegradationScores(scores map[int64]AccountDegradationScore, accountIDs []int64) map[int64]AccountDegradationScore {
	if len(scores) == 0 || len(accountIDs) == 0 {
		return nil
	}
	result := make(map[int64]AccountDegradationScore, len(accountIDs))
	for _, accountID := range accountIDs {
		if score, ok := scores[accountID]; ok {
			result[accountID] = score
		}
	}
	return result
}

func accountSchedulerStatKey(groupID, accountID int64) string {
	return strconv.FormatInt(groupID, 10) + ":" + strconv.FormatInt(accountID, 10)
}

func normalizeAccountSchedulerReasoningEffort(model, effort string) string {
	normalized := normalizeOpenAIReasoningEffortForModel(strings.TrimSpace(effort), model)
	if normalized == "" {
		return "medium"
	}
	return normalized
}

func firstAccountSchedulerModel(models []string) string {
	for _, model := range models {
		value := strings.TrimSpace(model)
		if value == "" || !isAccountSchedulerProbeModel(value) {
			continue
		}
		if strings.Contains(strings.ToLower(value), "-sol") {
			return value
		}
	}
	for _, model := range models {
		value := strings.TrimSpace(model)
		if value != "" && isAccountSchedulerProbeModel(value) {
			return value
		}
	}
	return ""
}

func (s *AccountSchedulerService) accountSchedulerGroupPrimaryModels(
	ctx context.Context,
	groups []AccountSchedulerGroup,
) map[int64]string {
	reader, ok := s.repo.(accountSchedulerProbeProfileReader)
	if !ok || len(groups) == 0 {
		return map[int64]string{}
	}
	groupIDs := make([]int64, 0, len(groups))
	for _, group := range groups {
		if group.Platform == PlatformOpenAI {
			groupIDs = append(groupIDs, group.ID)
		}
	}
	models, err := reader.ListGroupPrimaryModels(ctx, groupIDs)
	if err != nil {
		logger.LegacyPrintf("service.account_scheduler", "[AccountScheduler] load monitor primary models failed: %v", err)
		return map[int64]string{}
	}
	return models
}

func resolveAccountSchedulerProbeModel(
	setting AccountSchedulerGroupSetting,
	group AccountSchedulerGroup,
	inheritedModel string,
) string {
	if model := strings.TrimSpace(setting.TestModel); model != "" {
		return model
	}
	for _, model := range group.ModelCandidates {
		if strings.EqualFold(strings.TrimSpace(model), accountSchedulerDefaultTestModel) {
			return model
		}
	}
	if model := strings.TrimSpace(inheritedModel); model != "" {
		return model
	}
	return firstAccountSchedulerModel(group.ModelCandidates)
}

func isAccountSchedulerProbeModel(model string) bool {
	lower := strings.ToLower(strings.TrimSpace(model))
	if lower == "" {
		return false
	}
	for _, excluded := range []string{
		"codex-auto-review",
		"embedding",
		"tts",
		"whisper",
		"audio",
		"image",
		"realtime",
	} {
		if strings.Contains(lower, excluded) {
			return false
		}
	}
	return strings.HasPrefix(lower, "gpt-")
}

func accountSchedulerGroupSupportsDegradation(group AccountSchedulerGroup) bool {
	return strings.EqualFold(strings.TrimSpace(group.Platform), PlatformOpenAI)
}

func rankAccountSchedulerRowsByAvailability(rows []AccountSchedulerRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Available != rows[j].Available {
			return rows[i].Available
		}
		return rows[i].ID < rows[j].ID
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
}

func findAccountSchedulerGroup(groups []AccountSchedulerGroup, groupID int64) *AccountSchedulerGroup {
	for i := range groups {
		if groups[i].ID == groupID {
			return &groups[i]
		}
	}
	return nil
}
