package upstreammonitor

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"sort"
	"strconv"
	"strings"
)

const siteColumns = `id,base_url,provider_type,account_id,display_name,enabled,warning_enabled,threshold::text,recharge_url,auth_cipher,upstream_user_id,amount::text,currency,status,error_code,last_success_at,last_attempt_at,next_check_at,alert_active,revision,failure_count,recovery_count,episode_id`

type scanner interface{ Scan(...any) error }

func scanSite(row scanner) (Site, error) {
	var s Site
	var threshold, amount sql.NullString
	var success, attempt, next sql.NullTime
	e := row.Scan(&s.ID, &s.BaseURL, &s.Provider, &s.AccountID, &s.Name, &s.Enabled, &s.WarningEnabled, &threshold, &s.RechargeURL, &s.authCipher, &s.UserID, &amount, &s.Currency, &s.Status, &s.ErrorCode, &success, &attempt, &next, &s.AlertActive, &s.Revision, &s.failureCount, &s.recoveryCount, &s.episodeID)
	if e != nil {
		return s, e
	}
	// Alerts are enabled for every monitored account; a missing threshold only collects balance.
	s.WarningEnabled = true
	if threshold.Valid {
		normalized, err := ParseAmount(threshold.String)
		if err != nil {
			return s, err
		}
		s.Threshold = &normalized
	}
	if amount.Valid {
		normalized, err := ParseAmount(amount.String)
		if err != nil {
			return s, err
		}
		s.Amount = &normalized
	}
	if success.Valid {
		s.LastSuccess = &success.Time
	}
	if attempt.Valid {
		s.LastAttempt = &attempt.Time
	}
	if next.Valid {
		s.NextCheck = &next.Time
	}
	s.HasAccessToken = s.authCipher != ""
	s.Groups = []Group{}
	return s, nil
}
func candidateID(base string) string {
	hash := sha256.Sum256([]byte(base))
	return hex.EncodeToString(hash[:12])
}
func (s *Service) event(ctx context.Context, site *string, kind, level, message string, details any) error {
	raw, e := json.Marshal(details)
	if e != nil {
		return e
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO upstream_balance_events(site_id,kind,level,message,details) VALUES($1,$2,$3,$4,$5)`, site, kind, level, message, raw)
	return e
}
func eventTx(ctx context.Context, tx *sql.Tx, site *string, kind, level, message string, details any) error {
	raw, e := json.Marshal(details)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO upstream_balance_events(site_id,kind,level,message,details) VALUES($1,$2,$3,$4,$5)`, site, kind, level, message, raw)
	return e
}
func (s *Service) Candidates(ctx context.Context) ([]Candidate, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT a.id,a.name,a.platform,a.status,COALESCE(a.credentials->>'base_url',''),COALESCE((SELECT json_agg(json_build_object('id',g.id,'name',g.name)) FROM account_groups ag JOIN groups g ON g.id=ag.group_id WHERE ag.account_id=a.id AND g.deleted_at IS NULL),'[]') FROM accounts a WHERE a.type='apikey' AND a.deleted_at IS NULL ORDER BY (a.status='active') DESC,a.id LIMIT 5000`)
	if e != nil {
		return nil, e
	}
	byURL := map[string]*Candidate{}
	result := []Candidate{}
	for rows.Next() {
		var a Account
		var raw string
		var groupJSON []byte
		if e = rows.Scan(&a.ID, &a.Name, &a.Platform, &a.Status, &raw, &groupJSON); e != nil {
			rows.Close()
			return nil, e
		}
		base, err := CanonicalURL(raw)
		if err != nil {
			continue
		}
		c := byURL[base]
		if c == nil {
			c = &Candidate{ID: candidateID(base), BaseURL: base, Accounts: []Account{}, Groups: []Group{}}
			byURL[base] = c
		}
		c.Accounts = append(c.Accounts, a)
		var gs []Group
		_ = json.Unmarshal(groupJSON, &gs)
		for _, g := range gs {
			exists := false
			for _, old := range c.Groups {
				exists = exists || old.ID == g.ID
			}
			if !exists {
				c.Groups = append(c.Groups, g)
			}
		}
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return nil, e
	}
	rows.Close()
	// Product hints come only from prior public discovery or an explicitly imported site.
	hints, e := s.DB.QueryContext(ctx, `SELECT DISTINCT ON (details->>'base_url') details->>'base_url',details->>'provider_type' FROM upstream_balance_events WHERE kind='identify' ORDER BY details->>'base_url',id DESC`)
	if e != nil {
		return nil, e
	}
	for hints.Next() {
		var base string
		var kind sql.NullString
		if e = hints.Scan(&base, &kind); e != nil {
			hints.Close()
			return nil, e
		}
		if c := byURL[base]; c != nil && (kind.String == "sub2api" || kind.String == "newapi") {
			c.Provider = kind.String
		}
	}
	e = hints.Err()
	hints.Close()
	if e != nil {
		return nil, e
	}
	monitored, e := s.DB.QueryContext(ctx, `SELECT base_url,provider_type FROM upstream_balance_sites WHERE deleted_at IS NULL`)
	if e != nil {
		return nil, e
	}
	for monitored.Next() {
		var base, kind string
		if e = monitored.Scan(&base, &kind); e != nil {
			monitored.Close()
			return nil, e
		}
		if c := byURL[base]; c != nil {
			c.Monitored = true
			c.Provider = kind
		}
	}
	e = monitored.Err()
	monitored.Close()
	if e != nil {
		return nil, e
	}
	for _, c := range byURL {
		sort.Slice(c.Groups, func(i, j int) bool { return c.Groups[i].ID < c.Groups[j].ID })
		result = append(result, *c)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].BaseURL < result[j].BaseURL })
	return result, nil
}
func (s *Service) List(ctx context.Context) ([]Site, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT `+siteColumns+` FROM upstream_balance_sites WHERE deleted_at IS NULL ORDER BY created_at,id`)
	if e != nil {
		return nil, e
	}
	sites := []Site{}
	for rows.Next() {
		site, e := scanSite(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		sites = append(sites, site)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	candidates, e := s.Candidates(ctx)
	if e != nil {
		return nil, e
	}
	for i := range sites {
		for _, c := range candidates {
			if c.BaseURL == sites[i].BaseURL {
				sites[i].Groups = c.Groups
				break
			}
		}
	}
	return sites, nil
}
func (s *Service) site(ctx context.Context, id string) (Site, error) {
	v, e := scanSite(s.DB.QueryRowContext(ctx, `SELECT `+siteColumns+` FROM upstream_balance_sites WHERE id=$1 AND deleted_at IS NULL`, id))
	if errors.Is(e, sql.ErrNoRows) {
		return v, Fail("MONITOR_NOT_FOUND")
	}
	return v, e
}
func validateInput(in *SiteInput) error {
	if in.Provider != "sub2api" && in.Provider != "newapi" {
		return Fail("MONITOR_INVALID_PROVIDER")
	}
	if in.Threshold != nil && strings.TrimSpace(*in.Threshold) != "" {
		amount, e := ParseAmount(strings.TrimSpace(*in.Threshold))
		if e != nil {
			return e
		}
		in.Threshold = &amount
	} else {
		threshold := DefaultThreshold
		in.Threshold = &threshold
	}
	// Retain the input field for older clients, but remove the separate alert opt-in.
	in.WarningEnabled = true
	if len(in.AccessToken) > 8192 || strings.ContainsAny(in.AccessToken, "\r\n\x00") {
		return Fail("MONITOR_INVALID_CREDENTIAL")
	}
	if in.UserID != "" {
		id, e := strconv.ParseInt(in.UserID, 10, 64)
		if e != nil || id <= 0 {
			return Fail("MONITOR_INVALID_UPSTREAM_USER")
		}
	}
	return nil
}
func (s *Service) Add(ctx context.Context, in SiteInput) (Site, error) {
	if e := validateInput(&in); e != nil {
		return Site{}, e
	}
	candidates, e := s.Candidates(ctx)
	if e != nil {
		return Site{}, e
	}
	var chosen *Candidate
	for i := range candidates {
		if candidates[i].ID == in.CandidateID {
			chosen = &candidates[i]
			break
		}
	}
	if chosen == nil {
		return Site{}, Fail("MONITOR_CANDIDATE_NOT_FOUND")
	}
	valid := false
	for _, a := range chosen.Accounts {
		valid = valid || a.ID == in.AccountID
	}
	if !valid {
		return Site{}, Fail("MONITOR_ACCOUNT_BINDING_CHANGED")
	}
	link := in.RechargeURL
	if link == "" {
		link = chosen.BaseURL
	}
	link, e = RechargeURL(link)
	if e != nil {
		return Site{}, e
	}
	encrypted, e := seal(s.master, strings.TrimSpace(in.AccessToken))
	if e != nil {
		return Site{}, e
	}
	id := uuid.NewString()
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return Site{}, e
	}
	defer tx.Rollback()
	var returned string
	e = tx.QueryRowContext(ctx, `INSERT INTO upstream_balance_sites(id,base_url,provider_type,account_id,display_name,enabled,warning_enabled,threshold,recharge_url,auth_cipher,upstream_user_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(base_url) DO UPDATE SET provider_type=EXCLUDED.provider_type,account_id=EXCLUDED.account_id,display_name=EXCLUDED.display_name,enabled=EXCLUDED.enabled,warning_enabled=EXCLUDED.warning_enabled,threshold=EXCLUDED.threshold,recharge_url=EXCLUDED.recharge_url,auth_cipher=EXCLUDED.auth_cipher,upstream_user_id=EXCLUDED.upstream_user_id,amount=NULL,currency='',status='pending',error_code='',last_success_at=NULL,last_attempt_at=NULL,next_check_at=now(),alert_active=FALSE,episode_id='',recovery_count=0,failure_count=0,revision=upstream_balance_sites.revision+1,deleted_at=NULL,updated_at=now() WHERE upstream_balance_sites.deleted_at IS NOT NULL RETURNING id`, id, chosen.BaseURL, in.Provider, in.AccountID, cleanName(in.Name, chosen.BaseURL), in.Enabled, in.WarningEnabled, in.Threshold, link, encrypted, in.UserID).Scan(&returned)
	if errors.Is(e, sql.ErrNoRows) {
		return Site{}, Fail("MONITOR_ALREADY_ADDED")
	}
	if e != nil {
		return Site{}, e
	}
	id = returned
	if e = eventTx(ctx, tx, &id, "settings", "info", "已手动加入监控", map[string]any{"provider_type": in.Provider, "account_id": in.AccountID}); e != nil {
		return Site{}, e
	}
	if e = tx.Commit(); e != nil {
		return Site{}, e
	}
	s.Wake()
	return s.site(ctx, id)
}
func (s *Service) Update(ctx context.Context, id string, in SiteInput) (Site, error) {
	if e := validateInput(&in); e != nil {
		return Site{}, e
	}
	link, e := RechargeURL(in.RechargeURL)
	if e != nil {
		return Site{}, e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return Site{}, e
	}
	defer tx.Rollback()
	old, e := scanSite(tx.QueryRowContext(ctx, `SELECT `+siteColumns+` FROM upstream_balance_sites WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id))
	if errors.Is(e, sql.ErrNoRows) {
		return Site{}, Fail("MONITOR_NOT_FOUND")
	}
	if e != nil {
		return Site{}, e
	}
	if old.Revision != in.Revision {
		return Site{}, Fail("MONITOR_CONFIG_CHANGED")
	}
	if old.Provider != in.Provider {
		return Site{}, Fail("MONITOR_INVALID_PROVIDER")
	}
	// Credential rebinding is explicit and must still match the selected Base URL.
	if in.AccountID != old.AccountID {
		var base string
		if e = tx.QueryRowContext(ctx, `SELECT COALESCE(credentials->>'base_url','') FROM accounts WHERE id=$1 AND type='apikey' AND deleted_at IS NULL`, in.AccountID).Scan(&base); e != nil {
			return Site{}, Fail("MONITOR_ACCOUNT_BINDING_CHANGED")
		}
		canonical, e := CanonicalURL(base)
		if e != nil || canonical != old.BaseURL {
			return Site{}, Fail("MONITOR_ACCOUNT_BINDING_CHANGED")
		}
	}
	identityChanged := in.AccountID != old.AccountID || in.AccessToken != "" || in.ClearAccessToken || in.UserID != old.UserID
	auth := old.authCipher
	if in.ClearAccessToken {
		auth = ""
	}
	if in.AccessToken != "" {
		auth, e = seal(s.master, strings.TrimSpace(in.AccessToken))
		if e != nil {
			return Site{}, e
		}
	}
	_, e = tx.ExecContext(ctx, `UPDATE upstream_balance_sites SET account_id=$2,display_name=$3,enabled=$4,warning_enabled=$5,threshold=$6,recharge_url=$7,auth_cipher=$8,upstream_user_id=$9,revision=revision+1,next_check_at=now(),updated_at=now(),amount=CASE WHEN $10 THEN NULL ELSE amount END,currency=CASE WHEN $10 THEN '' ELSE currency END,alert_active=CASE WHEN $10 THEN FALSE ELSE alert_active END,episode_id=CASE WHEN $10 THEN '' ELSE episode_id END,recovery_count=0,status=CASE WHEN $10 THEN 'pending' ELSE status END,error_code=CASE WHEN $10 THEN '' ELSE error_code END WHERE id=$1`, id, in.AccountID, cleanName(in.Name, old.BaseURL), in.Enabled, in.WarningEnabled, in.Threshold, link, auth, in.UserID, identityChanged)
	if e != nil {
		return Site{}, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE upstream_balance_deliveries SET status='cancelled',updated_at=now() WHERE site_id=$1 AND NOT is_test AND status IN ('pending','retry','needs_config','awaiting_balance')`, id); e != nil {
		return Site{}, e
	}
	if e = eventTx(ctx, tx, &id, "settings", "info", "监控配置已更新", map[string]any{"old_threshold": old.Threshold, "threshold": in.Threshold, "warning_enabled": in.WarningEnabled, "enabled": in.Enabled, "credential_changed": identityChanged}); e != nil {
		return Site{}, e
	}
	if e = tx.Commit(); e != nil {
		return Site{}, e
	}
	s.Wake()
	return s.site(ctx, id)
}
func (s *Service) Remove(ctx context.Context, id string, revision int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	result, e := tx.ExecContext(ctx, `UPDATE upstream_balance_sites SET deleted_at=now(),enabled=FALSE,warning_enabled=FALSE,revision=revision+1,updated_at=now() WHERE id=$1 AND revision=$2 AND deleted_at IS NULL`, id, revision)
	if e != nil {
		return e
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return Fail("MONITOR_CONFIG_CHANGED")
	}
	if _, e = tx.ExecContext(ctx, `UPDATE upstream_balance_deliveries SET status='cancelled' WHERE site_id=$1 AND NOT is_test AND status IN ('pending','retry','needs_config','awaiting_balance')`, id); e != nil {
		return e
	}
	if e = eventTx(ctx, tx, &id, "settings", "info", "已移出监控，历史日志保留", map[string]any{}); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Service) Events(ctx context.Context, siteID, kind string, page int) (EventsPage, error) {
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		page = 10000
	}
	switch kind {
	case "", "identify", "query", "alert", "delivery", "settings":
	default:
		return EventsPage{}, Fail("MONITOR_INVALID_EVENT_KIND")
	}
	result := EventsPage{Items: []Event{}, Page: page, PageSize: 30}
	where := ` WHERE ($1='' OR site_id=$1) AND ($2='' OR kind=$2)`
	if e := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM upstream_balance_events`+where, siteID, kind).Scan(&result.Total); e != nil {
		return result, e
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT id,site_id,kind,level,message,details,created_at FROM upstream_balance_events`+where+` ORDER BY id DESC LIMIT 30 OFFSET $3`, siteID, kind, (page-1)*30)
	if e != nil {
		return result, e
	}
	defer rows.Close()
	for rows.Next() {
		var v Event
		if e = rows.Scan(&v.ID, &v.SiteID, &v.Kind, &v.Level, &v.Message, &v.Details, &v.CreatedAt); e != nil {
			return result, e
		}
		result.Items = append(result.Items, v)
	}
	return result, rows.Err()
}
func (s *Service) Channel(ctx context.Context) (Channel, error) {
	var c Channel
	var encrypted string
	e := s.DB.QueryRowContext(ctx, `SELECT app_id,chat_id,secret_cipher,revision FROM upstream_balance_channel WHERE id=1`).Scan(&c.AppID, &c.ChatID, &encrypted, &c.Revision)
	if errors.Is(e, sql.ErrNoRows) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	c.HasSecret = encrypted != ""
	c.secret, e = unseal(s.master, encrypted)
	return c, e
}
func (s *Service) SaveChannel(ctx context.Context, in ChannelInput) (Channel, error) {
	if !strings.HasPrefix(in.AppID, "cli_") || len(in.AppID) > 100 || !strings.HasPrefix(in.ChatID, "oc_") || len(in.ChatID) > 100 || len(in.Secret) > 4096 || strings.ContainsAny(in.Secret, "\r\n\x00") {
		return Channel{}, Fail("MONITOR_INVALID_FEISHU_CONFIG")
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return Channel{}, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(79310260)`); e != nil {
		return Channel{}, e
	}
	var oldApp, encrypted string
	var rev int64
	e = tx.QueryRowContext(ctx, `SELECT app_id,secret_cipher,revision FROM upstream_balance_channel WHERE id=1 FOR UPDATE`).Scan(&oldApp, &encrypted, &rev)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return Channel{}, e
	}
	if rev != in.Revision {
		return Channel{}, Fail("MONITOR_CONFIG_CHANGED")
	}
	if in.Secret != "" {
		encrypted, e = seal(s.master, in.Secret)
		if e != nil {
			return Channel{}, e
		}
	} else if encrypted == "" || oldApp != in.AppID {
		return Channel{}, Fail("MONITOR_FEISHU_SECRET_REQUIRED")
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO upstream_balance_channel(id,app_id,chat_id,secret_cipher) VALUES(1,$1,$2,$3) ON CONFLICT(id) DO UPDATE SET app_id=$1,chat_id=$2,secret_cipher=$3,revision=upstream_balance_channel.revision+1,updated_at=now()`, in.AppID, in.ChatID, encrypted)
	if e != nil {
		return Channel{}, e
	}
	if e = eventTx(ctx, tx, nil, "settings", "info", "飞书应用通知配置已更新", map[string]any{"chat_configured": true}); e != nil {
		return Channel{}, e
	}
	if e = tx.Commit(); e != nil {
		return Channel{}, e
	}
	return s.Channel(ctx)
}
