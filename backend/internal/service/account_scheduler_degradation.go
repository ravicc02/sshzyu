package service

import (
	"context"
	"errors"
	"math"
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
)

var ErrAccountSchedulerInvalidSetting = errors.New("invalid account scheduler setting")

type AccountSchedulerGroup struct {
	ID              int64
	Name            string
	Platform        string
	Status          string
	ModelCandidates []string
}

type AccountSchedulerGroupSetting struct {
	GroupID            int64
	DegradationEnabled bool
	IntervalSeconds    int
	TestModel          string
	UpdatedAt          time.Time
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
	ID           int64
	AccountID    int64
	AccountName  string
	GroupID      int64
	Model        string
	Status       string
	ResponseText string
	ErrorMessage string
	LatencyMs    int64
	CheckedAt    time.Time
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
	ID                 int64                 `json:"id"`
	Name               string                `json:"name"`
	Platform           string                `json:"platform"`
	Status             string                `json:"status"`
	DegradationEnabled bool                  `json:"degradation_enabled"`
	IntervalSeconds    int                   `json:"interval_seconds"`
	TestModel          string                `json:"test_model"`
	ModelCandidates    []string              `json:"model_candidates"`
	OAuthFirst         bool                  `json:"oauth_first"`
	LowestAccountID    *int64                `json:"lowest_account_id,omitempty"`
	Accounts           []AccountSchedulerRow `json:"accounts"`
}

type AccountSchedulerRow struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	Platform           string     `json:"platform"`
	CredentialType     string     `json:"credential_type"`
	OAuth              bool       `json:"oauth"`
	Available          bool       `json:"available"`
	AvailabilityReason string     `json:"availability_reason"`
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
	GroupID            int64    `json:"group_id"`
	DegradationEnabled *bool    `json:"degradation_enabled"`
	IntervalSeconds    *int     `json:"interval_seconds"`
	TestModel          *string  `json:"test_model"`
	DegradationWeight  *float64 `json:"degradation_weight"`
}

type AccountSchedulerRunInput struct {
	GroupID   int64 `json:"group_id"`
	AccountID int64 `json:"account_id,omitempty"`
}

type AccountSchedulerRunResult struct {
	Run     int                       `json:"run"`
	Skipped int                       `json:"skipped"`
	Failed  int                       `json:"failed"`
	Results []AccountDegradationProbe `json:"results"`
}

type AccountDegradationScore struct {
	Rate       float64
	HasSamples bool
	Samples    int64
}

type AccountDegradationScoreProvider interface {
	AccountDegradationScores(ctx context.Context, groupID int64, accountIDs []int64) map[int64]AccountDegradationScore
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
	expiresAt time.Time
	scores    map[int64]AccountDegradationScore
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
	overview := &AccountSchedulerOverview{
		ScoringMode:       "score",
		OAuthFirst:        true,
		PriorityIgnored:   true,
		DegradationWeight: weight,
		AdvancedEnabled:   advancedEnabled,
		GeneratedAt:       time.Now(),
	}

	for _, group := range groups {
		setting := settings[group.ID]
		if setting.GroupID == 0 {
			setting = AccountSchedulerGroupSetting{
				GroupID:         group.ID,
				IntervalSeconds: accountSchedulerDefaultIntervalSeconds,
				TestModel:       firstAccountSchedulerModel(group.ModelCandidates),
			}
		}
		if setting.IntervalSeconds <= 0 {
			setting.IntervalSeconds = accountSchedulerDefaultIntervalSeconds
		}
		if strings.TrimSpace(setting.TestModel) == "" {
			setting.TestModel = firstAccountSchedulerModel(group.ModelCandidates)
		}

		groupAccounts := accountsByGroup[group.ID]
		rows := make([]AccountSchedulerRow, 0, len(groupAccounts))
		oauthFirst := false
		availableCount := 0
		for _, account := range groupAccounts {
			stat := statsByKey[accountSchedulerStatKey(group.ID, account.ID)]
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
				SampleCount:        stat.SampleCount(),
				PassCount:          stat.PassCount,
				DegradedCount:      stat.DegradedCount,
				ErrorCount:         stat.ErrorCount,
				LatestStatus:       stat.LatestStatus,
				LatestCheckedAt:    stat.LatestCheckedAt,
				AverageLatencyMs:   stat.AverageLatencyMs,
				ScoreWeight:        weight,
			}
			if rate, ok := stat.DegradationRate(); ok {
				row.DegradationRate = &rate
				row.DegradationFactor = 1 - rate
			} else {
				row.DegradationFactor = 0.5
			}
			rows = append(rows, row)
		}

		s.applyScores(ctx, group.ID, groupAccounts, rows)
		markLowestAccountDegradation(rows)
		groupView := AccountSchedulerGroupView{
			ID:                 group.ID,
			Name:               group.Name,
			Platform:           group.Platform,
			Status:             group.Status,
			DegradationEnabled: setting.DegradationEnabled,
			IntervalSeconds:    setting.IntervalSeconds,
			TestModel:          setting.TestModel,
			ModelCandidates:    group.ModelCandidates,
			OAuthFirst:         oauthFirst,
			Accounts:           rows,
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
		if setting.DegradationEnabled {
			overview.EnabledGroups++
		}
		if len(stats) > 0 && len(rows) > 0 {
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
	settings, err := s.repo.ListGroupSettings(ctx)
	if err != nil {
		return nil, err
	}
	setting := settings[input.GroupID]
	if setting.GroupID == 0 {
		setting = AccountSchedulerGroupSetting{
			GroupID:         input.GroupID,
			IntervalSeconds: accountSchedulerDefaultIntervalSeconds,
		}
	}
	if input.DegradationEnabled != nil {
		setting.DegradationEnabled = *input.DegradationEnabled
	}
	if input.IntervalSeconds != nil {
		setting.IntervalSeconds = *input.IntervalSeconds
	}
	if input.TestModel != nil {
		setting.TestModel = strings.TrimSpace(*input.TestModel)
	}
	if setting.IntervalSeconds < accountSchedulerMinimumIntervalSeconds || setting.IntervalSeconds > accountSchedulerMaximumIntervalSeconds {
		return nil, ErrAccountSchedulerInvalidSetting
	}
	if input.DegradationWeight != nil {
		weight := *input.DegradationWeight
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight < 0 || weight > 100 {
			return nil, ErrAccountSchedulerInvalidSetting
		}
		if err := s.settingRepo.SetMultiple(ctx, map[string]string{
			SettingKeyOpenAIAdvancedSchedulerWeightDegradation: strconv.FormatFloat(weight, 'f', -1, 64),
		}); err != nil {
			return nil, err
		}
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
	for _, account := range accounts {
		if input.AccountID > 0 && account.ID != input.AccountID {
			continue
		}
		setting := settings[account.GroupID]
		if input.AccountID <= 0 && !setting.DegradationEnabled {
			result.Skipped++
			continue
		}
		group := findAccountSchedulerGroup(groups, account.GroupID)
		model := strings.TrimSpace(setting.TestModel)
		if model == "" && group != nil {
			model = firstAccountSchedulerModel(group.ModelCandidates)
		}
		probe, err := s.runProbe(ctx, account, model)
		if err != nil {
			result.Failed++
			continue
		}
		result.Run++
		result.Results = append(result.Results, *probe)
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
		if setting.DegradationEnabled {
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
		model := strings.TrimSpace(setting.TestModel)
		if model == "" && group != nil {
			model = firstAccountSchedulerModel(group.ModelCandidates)
		}
		probe, err := s.runProbe(ctx, account, model)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			result.Failed++
			return
		}
		result.Run++
		result.Results = append(result.Results, *probe)
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
	now := time.Now()
	s.cacheMu.RLock()
	entry, ok := s.cache[groupID]
	s.cacheMu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return filterAccountDegradationScores(entry.scores, accountIDs)
	}

	stats, err := s.repo.ListDegradationStats(ctx, now.Add(-accountSchedulerHistoryWindow))
	if err != nil {
		return nil
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
	s.cacheMu.Lock()
	s.cache[groupID] = accountDegradationScoreCacheEntry{
		expiresAt: now.Add(30 * time.Second),
		scores:    scores,
	}
	s.cacheMu.Unlock()
	return filterAccountDegradationScores(scores, accountIDs)
}

func (s *AccountSchedulerService) runProbe(ctx context.Context, account AccountSchedulerAccount, model string) (*AccountDegradationProbe, error) {
	startedAt := time.Now()
	probe := &AccountDegradationProbe{
		AccountID:   account.ID,
		AccountName: account.Name,
		GroupID:     account.GroupID,
		Model:       model,
		Status:      AccountDegradationStatusError,
		CheckedAt:   startedAt,
	}
	result, err := s.accountTestSvc.RunDegradationProbe(ctx, account.ID, model, monitorIntelligencePrompt)
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

func (s *AccountSchedulerService) degradationWeight(ctx context.Context) (float64, error) {
	if s.settingRepo == nil {
		return 1.5, nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyOpenAIAdvancedSchedulerWeightDegradation})
	if err != nil {
		return 1.5, err
	}
	raw := strings.TrimSpace(values[SettingKeyOpenAIAdvancedSchedulerWeightDegradation])
	if raw == "" {
		return 1.5, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
		return 1.5, nil
	}
	return value, nil
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

func firstAccountSchedulerModel(models []string) string {
	for _, model := range models {
		if value := strings.TrimSpace(model); value != "" {
			return value
		}
	}
	return ""
}

func findAccountSchedulerGroup(groups []AccountSchedulerGroup, groupID int64) *AccountSchedulerGroup {
	for i := range groups {
		if groups[i].ID == groupID {
			return &groups[i]
		}
	}
	return nil
}
