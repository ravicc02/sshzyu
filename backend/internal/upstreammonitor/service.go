package upstreammonitor

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"sync"
	"time"
)

type Service struct {
	DB        *sql.DB
	Fetcher   Fetcher
	Sender    Sender
	master    string
	slots     chan struct{}
	sendSlots chan struct{}
	wake      chan struct{}
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	start     sync.Once
	stop      sync.Once
}

func New(db *sql.DB, fetcher Fetcher, sender Sender, master string) *Service {
	return &Service{DB: db, Fetcher: fetcher, Sender: sender, master: master, slots: make(chan struct{}, 2), sendSlots: make(chan struct{}, 1), wake: make(chan struct{}, 1)}
}
func (s *Service) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *Service) Start(parent context.Context) {
	s.start.Do(func() {
		ctx, cancel := context.WithCancel(parent)
		s.cancel = cancel
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				case <-s.wake:
				}
				s.RunDue(ctx)
				s.DispatchDue(ctx)
			}
		}()
	})
}
func (s *Service) Stop() {
	s.stop.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.wg.Wait()
	})
}
func (s *Service) RunDue(ctx context.Context) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id FROM upstream_balance_sites WHERE deleted_at IS NULL AND enabled AND next_check_at<=now() ORDER BY next_check_at LIMIT 20`)
	if e != nil {
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	var wg sync.WaitGroup
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(id string) { defer wg.Done(); _, _ = s.Refresh(ctx, id) }(id)
	}
	wg.Wait()
}
func (s *Service) Refresh(ctx context.Context, id string) (Site, error) {
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return Site{}, ctx.Err()
	}
	defer func() { <-s.slots }()
	conn, e := s.DB.Conn(ctx)
	if e != nil {
		return Site{}, e
	}
	defer conn.Close()
	var locked bool
	if e = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,79310259))`, id).Scan(&locked); e != nil {
		return Site{}, e
	}
	if !locked {
		return Site{}, Fail("MONITOR_QUERY_BUSY")
	}
	defer func() {
		release, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_, _ = conn.ExecContext(release, `SELECT pg_advisory_unlock(hashtextextended($1,79310259))`, id)
	}()
	site, e := s.site(ctx, id)
	if e != nil {
		return Site{}, e
	}
	queryCtx, stopQuery := context.WithTimeout(ctx, 22*time.Second)
	reading, queryErr := s.fetch(queryCtx, site)
	stopQuery()
	if ctx.Err() != nil {
		return Site{}, ctx.Err()
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return Site{}, e
	}
	defer tx.Rollback()
	current, e := scanSite(tx.QueryRowContext(ctx, `SELECT `+siteColumns+` FROM upstream_balance_sites WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id))
	if e != nil {
		return Site{}, e
	}
	if current.Revision != site.Revision {
		return Site{}, Fail("MONITOR_CONFIG_CHANGED")
	}
	if queryErr != nil {
		code := Code(queryErr)
		status := "error"
		var next any = time.Now().Add(time.Minute * time.Duration(1<<min(current.failureCount+1, 6)))
		switch code {
		case "MONITOR_ACCOUNT_CREDENTIAL_REQUIRED", "MONITOR_AUTH_REQUIRED", "MONITOR_API_REJECTED":
			status = "needs_configuration"
			next = nil
		case "MONITOR_ACCOUNT_BALANCE_NOT_EXPOSED", "MONITOR_ENDPOINT_UNAVAILABLE", "MONITOR_BALANCE_MISSING", "MONITOR_CURRENCY_UNCONFIRMED", "MONITOR_CURRENCY_MISSING":
			status = "unsupported"
			next = nil
		}
		var failure *Failure
		if errors.As(queryErr, &failure) && failure.RetryAfter > 0 {
			next = time.Now().Add(failure.RetryAfter)
		}
		_, e = tx.ExecContext(ctx, `UPDATE upstream_balance_sites SET status=$2,error_code=$3,last_attempt_at=now(),next_check_at=$4,failure_count=failure_count+1,recovery_count=0,updated_at=now() WHERE id=$1`, id, status, code, next)
		if e != nil {
			return Site{}, e
		}
		if e = eventTx(ctx, tx, &id, "query", "warning", "总余额查询未完成", map[string]any{"error_code": code, "status": status}); e != nil {
			return Site{}, e
		}
	} else {
		active := current.AlertActive
		episode := current.episodeID
		recoveries := 0
		warning := true
		currencyChanged := current.Currency != "" && current.Currency != reading.Currency
		if currencyChanged {
			current.Threshold = nil
			active = false
			episode = ""
			if e = eventTx(ctx, tx, &id, "settings", "warning", "余额币种变化，请重新设置预警阈值", map[string]any{"old_currency": current.Currency, "currency": reading.Currency}); e != nil {
				return Site{}, e
			}
		}
		low := current.Enabled && warning && current.Threshold != nil && Below(reading.Amount, *current.Threshold)
		if low {
			if !active {
				active = true
				episode = uuid.NewString()
				if e = eventTx(ctx, tx, &id, "alert", "warning", "账户总余额低于预警阈值", map[string]any{"amount": reading.Amount, "currency": reading.Currency, "threshold": current.Threshold, "episode_id": episode}); e != nil {
					return Site{}, e
				}
			}
			if e = enqueueTx(ctx, tx, current, reading, episode); e != nil {
				return Site{}, e
			}
		} else if active {
			if !current.Enabled || !warning || current.Threshold == nil {
				active = false
			} else {
				recoveries = current.recoveryCount + 1
				if recoveries >= 2 {
					active = false
					if e = eventTx(ctx, tx, &id, "alert", "info", "账户总余额已恢复", map[string]any{"amount": reading.Amount, "currency": reading.Currency, "episode_id": episode}); e != nil {
						return Site{}, e
					}
				}
			}
		}
		if !active {
			_, e = tx.ExecContext(ctx, `UPDATE upstream_balance_deliveries SET status='cancelled',updated_at=now() WHERE site_id=$1 AND NOT is_test AND status IN ('pending','retry','needs_config','awaiting_balance')`, id)
			if e != nil {
				return Site{}, e
			}
		}
		_, e = tx.ExecContext(ctx, `UPDATE upstream_balance_sites SET amount=$2,currency=$3,status='ok',error_code='',last_success_at=now(),last_attempt_at=now(),next_check_at=now()+interval '5 minutes',failure_count=0,alert_active=$4,episode_id=$5,recovery_count=$6,warning_enabled=TRUE,threshold=$7,updated_at=now() WHERE id=$1`, id, reading.Amount, reading.Currency, active, episode, recoveries, current.Threshold)
		if e != nil {
			return Site{}, e
		}
		if e = eventTx(ctx, tx, &id, "query", "info", "已读取账户总余额", map[string]any{"amount": reading.Amount, "currency": reading.Currency}); e != nil {
			return Site{}, e
		}
	}
	if e = tx.Commit(); e != nil {
		return Site{}, e
	}
	s.Wake()
	return s.site(ctx, id)
}
func enqueueTx(ctx context.Context, tx *sql.Tx, site Site, reading Reading, episode string) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO upstream_balance_deliveries(id,site_id,episode_id,site_revision,title,amount,currency,recharge_url,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,CASE WHEN EXISTS(SELECT 1 FROM upstream_balance_channel WHERE id=1) THEN 'pending' ELSE 'needs_config' END) ON CONFLICT(site_id,episode_id) DO UPDATE SET site_revision=EXCLUDED.site_revision,title=EXCLUDED.title,amount=EXCLUDED.amount,currency=EXCLUDED.currency,recharge_url=EXCLUDED.recharge_url,status=EXCLUDED.status,next_attempt_at=now(),updated_at=now() WHERE upstream_balance_deliveries.status IN ('cancelled','awaiting_balance') OR (upstream_balance_deliveries.status='needs_config' AND EXCLUDED.status='pending')`, uuid.NewString(), site.ID, episode, site.Revision, site.Name, reading.Amount, reading.Currency, site.RechargeURL)
	return e
}

const deliveryColumns = `id,site_id,episode_id,site_revision,title,amount,currency,recharge_url,is_test,status,attempts,message_id,error_code`

func scanDelivery(row scanner) (Delivery, error) {
	var d Delivery
	e := row.Scan(&d.ID, &d.SiteID, &d.EpisodeID, &d.Revision, &d.Name, &d.Amount, &d.Currency, &d.RechargeURL, &d.Test, &d.Status, &d.Attempts, &d.MessageID, &d.ErrorCode)
	return d, e
}
func (s *Service) DispatchDue(ctx context.Context) {
	s.expireDeliveries(ctx)
	for i := 0; i < 10 && ctx.Err() == nil; i++ {
		d, e := s.dispatch(ctx, "")
		if e != nil || d.ID == "" {
			return
		}
	}
}
func (s *Service) dispatch(ctx context.Context, id string) (Delivery, error) {
	select {
	case s.sendSlots <- struct{}{}:
	case <-ctx.Done():
		return Delivery{}, ctx.Err()
	}
	defer func() { <-s.sendSlots }()
	d, e := scanDelivery(s.DB.QueryRowContext(ctx, `WITH chosen AS (SELECT id FROM upstream_balance_deliveries WHERE ($1='' OR id=$1) AND ((status IN ('pending','retry') AND next_attempt_at<=now()) OR (status='sending' AND lease_until<now())) AND created_at>now()-interval '45 minutes' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE upstream_balance_deliveries d SET status='sending',lease_until=now()+interval '60 seconds',attempts=attempts+1,updated_at=now() FROM chosen WHERE d.id=chosen.id RETURNING d.`+`id,d.site_id,d.episode_id,d.site_revision,d.title,d.amount,d.currency,d.recharge_url,d.is_test,d.status,d.attempts,d.message_id,d.error_code`, id))
	if errors.Is(e, sql.ErrNoRows) {
		return Delivery{}, nil
	}
	if e != nil {
		return d, e
	}
	if !d.Test && d.SiteID != nil {
		site, e := s.site(ctx, *d.SiteID)
		if e != nil || !site.Enabled || !site.WarningEnabled || !site.AlertActive || site.episodeID != d.EpisodeID || site.Revision != d.Revision {
			return s.finishDelivery(ctx, d, "cancelled", "MONITOR_ALERT_CHANGED", "")
		}
		if site.LastSuccess == nil || time.Since(*site.LastSuccess) > 10*time.Minute || site.Status != "ok" {
			return s.finishDelivery(ctx, d, "awaiting_balance", "MONITOR_BALANCE_STALE", "")
		}
	}
	cfg, e := s.Channel(ctx)
	if e != nil {
		return s.finishDelivery(ctx, d, "failed", "MONITOR_SECRET_DECRYPT_FAILED", "")
	}
	if !cfg.HasSecret || cfg.AppID == "" || cfg.ChatID == "" {
		return s.finishDelivery(ctx, d, "needs_config", "MONITOR_FEISHU_NOT_CONFIGURED", "")
	}
	if s.Sender == nil {
		return s.finishDelivery(ctx, d, "failed", "MONITOR_TRANSPORT_UNAVAILABLE", "")
	}
	sendCtx, stop := context.WithTimeout(ctx, 26*time.Second)
	message, e := s.Sender.Send(sendCtx, cfg, d)
	stop()
	if e == nil {
		return s.finishDelivery(ctx, d, "sent", "", message)
	}
	status := "failed"
	code := Code(e)
	var failure *Failure
	if errors.As(e, &failure) && failure.Retryable && d.Attempts < 4 {
		status = "retry"
		delay := time.Duration(d.Attempts*d.Attempts) * 10 * time.Second
		if failure.RetryAfter > delay {
			delay = failure.RetryAfter
		}
		if delay > 10*time.Minute {
			delay = 10 * time.Minute
		}
		_, _ = s.DB.ExecContext(context.WithoutCancel(ctx), `UPDATE upstream_balance_deliveries SET next_attempt_at=now()+$2*interval '1 second' WHERE id=$1`, d.ID, int64(delay/time.Second))
	}
	return s.finishDelivery(context.WithoutCancel(ctx), d, status, code, "")
}
func (s *Service) finishDelivery(ctx context.Context, d Delivery, status, code, messageID string) (Delivery, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return d, e
	}
	defer tx.Rollback()
	result, updateErr := tx.ExecContext(ctx, `UPDATE upstream_balance_deliveries SET status=$2,error_code=$3,message_id=$4,lease_until=NULL,updated_at=now() WHERE id=$1 AND status='sending' AND attempts=$5`, d.ID, status, code, messageID, d.Attempts)
	e = updateErr
	if e != nil {
		return d, e
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return d, Fail("MONITOR_DELIVERY_STALE")
	}
	level := "info"
	if status != "sent" && status != "cancelled" {
		level = "warning"
	}
	if e = eventTx(ctx, tx, d.SiteID, "delivery", level, "飞书消息投递："+status, map[string]any{"delivery_id": d.ID, "status": status, "error_code": code, "attempt": d.Attempts, "message_id": messageID, "is_test": d.Test}); e != nil {
		return d, e
	}
	if e = tx.Commit(); e != nil {
		return d, e
	}
	d.Status = status
	d.ErrorCode = code
	d.MessageID = messageID
	return d, nil
}
func (s *Service) TestNotification(ctx context.Context, requestID, siteID string) (Delivery, error) {
	if _, e := uuid.Parse(requestID); e != nil {
		return Delivery{}, Fail("MONITOR_INVALID_REQUEST_ID")
	}
	d := Delivery{ID: requestID, EpisodeID: requestID, Name: "本地测试 · 上游余额预警", Amount: "8.50", Currency: "USD", RechargeURL: "https://example.com", Test: true}
	if siteID != "" {
		site, e := s.site(ctx, siteID)
		if e != nil {
			return d, e
		}
		if site.Amount == nil || site.LastSuccess == nil || time.Since(*site.LastSuccess) > 10*time.Minute || site.Status != "ok" {
			return d, Fail("MONITOR_BALANCE_STALE")
		}
		d.SiteID = &site.ID
		d.Revision = site.Revision
		d.Name = "本地测试 · " + site.Name
		d.Amount = *site.Amount
		d.Currency = site.Currency
		d.RechargeURL = site.RechargeURL
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return d, e
	}
	defer tx.Rollback()
	result, e := tx.ExecContext(ctx, `INSERT INTO upstream_balance_deliveries(id,site_id,episode_id,site_revision,title,amount,currency,recharge_url,is_test) VALUES($1,$2,$3,$4,$5,$6,$7,$8,TRUE) ON CONFLICT(id) DO NOTHING`, d.ID, d.SiteID, d.EpisodeID, d.Revision, d.Name, d.Amount, d.Currency, d.RechargeURL)
	if e != nil {
		return d, e
	}
	n, _ := result.RowsAffected()
	if n == 1 {
		if e = eventTx(ctx, tx, d.SiteID, "delivery", "info", "管理员手动发起测试消息", map[string]any{"delivery_id": d.ID, "is_test": true}); e != nil {
			return d, e
		}
	}
	if e = tx.Commit(); e != nil {
		return d, e
	}
	existing, e := scanDelivery(s.DB.QueryRowContext(ctx, `SELECT `+deliveryColumns+` FROM upstream_balance_deliveries WHERE id=$1`, requestID))
	if e != nil {
		return d, e
	}
	if !existing.Test {
		return d, Fail("MONITOR_INVALID_REQUEST_ID")
	}
	if existing.Status == "sent" || existing.Status == "failed" || existing.Status == "needs_config" {
		return existing, nil
	}
	sent, e := s.dispatch(ctx, requestID)
	if sent.ID == "" && e == nil {
		return existing, nil
	}
	return sent, e
}

// Expired tasks cannot remain pending forever or outlive the provider's retry budget.
func (s *Service) expireDeliveries(ctx context.Context) {
	rows, err := s.DB.QueryContext(ctx, `WITH expired AS (SELECT id FROM upstream_balance_deliveries WHERE status IN ('pending','retry','sending') AND created_at<=now()-interval '45 minutes' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 20) UPDATE upstream_balance_deliveries d SET status='failed',error_code='MONITOR_DELIVERY_EXPIRED',lease_until=NULL,updated_at=now() FROM expired WHERE d.id=expired.id RETURNING d.id,d.site_id`)
	if err != nil {
		return
	}
	type expired struct {
		id   string
		site *string
	}
	items := []expired{}
	for rows.Next() {
		var item expired
		if rows.Scan(&item.id, &item.site) == nil {
			items = append(items, item)
		}
	}
	rows.Close()
	for _, item := range items {
		_ = s.event(ctx, item.site, "delivery", "warning", "投递任务已超过重试时限", map[string]any{"delivery_id": item.id, "status": "failed", "error_code": "MONITOR_DELIVERY_EXPIRED"})
	}
}
