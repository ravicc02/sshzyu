package upstreammonitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fetchFunc func(context.Context, int64, string, string, Credential) (*http.Response, error)

func (f fetchFunc) Get(c context.Context, id int64, b, p string, k Credential) (*http.Response, error) {
	return f(c, id, b, p, k)
}

type fakeSender struct {
	count   atomic.Int32
	failure error
}

func (f *fakeSender) Send(context.Context, Channel, Delivery) (string, error) {
	f.count.Add(1)
	return "om_fixture", f.failure
}
func setup(t *testing.T) (*Service, *fakeSender) {
	t.Helper()
	dsn := os.Getenv("UPSTREAM_MONITOR_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	parent, e := sql.Open("postgres", dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := "monitor_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = parent.Exec(`CREATE SCHEMA ` + schema); e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, e := sql.Open("postgres", u.String())
	if e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(8)
	t.Cleanup(func() { db.Close(); parent.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); parent.Close() })
	fixture := `CREATE TABLE accounts(id BIGINT PRIMARY KEY,name TEXT,platform TEXT,status TEXT,type TEXT,credentials JSONB,deleted_at TIMESTAMPTZ);CREATE TABLE groups(id BIGINT PRIMARY KEY,name TEXT,deleted_at TIMESTAMPTZ);CREATE TABLE account_groups(account_id BIGINT,group_id BIGINT);INSERT INTO groups VALUES(1,'group one',NULL),(2,'group two',NULL);INSERT INTO accounts VALUES(1,'one','openai','active','apikey','{"base_url":"https://example.com"}',NULL),(2,'two','deepseek','active','apikey','{"base_url":"https://example.com/"}',NULL),(3,'other','openai','active','apikey','{"base_url":"https://other.invalid"}',NULL);INSERT INTO account_groups VALUES(1,1),(1,2),(2,1);`
	if _, e = db.Exec(fixture); e != nil {
		t.Fatal(e)
	}
	migration, e := os.ReadFile(filepath.Join("..", "..", "migrations", "258_upstream_balance_monitor.sql"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(migration)); e != nil {
		t.Fatal(e)
	}
	sender := &fakeSender{}
	s := New(db, nil, sender, "fixture-master")
	return s, sender
}
func wallet(value string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"mode":"unrestricted","balance":` + value + `,"unit":"USD"}`))}
}
func addSite(t *testing.T, s *Service, warning bool, threshold *string) Site {
	t.Helper()
	ctx := context.Background()
	cs, e := s.Candidates(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var c Candidate
	for _, v := range cs {
		if v.BaseURL == "https://example.com" {
			c = v
		}
	}
	site, e := s.Add(ctx, SiteInput{CandidateID: c.ID, Provider: "sub2api", AccountID: 1, Name: "fixture upstream", Enabled: true, WarningEnabled: warning, Threshold: threshold, RechargeURL: c.BaseURL})
	if e != nil {
		t.Fatal(e)
	}
	return site
}
func configure(t *testing.T, s *Service) {
	t.Helper()
	if _, e := s.SaveChannel(context.Background(), ChannelInput{AppID: "cli_fixture", ChatID: "oc_fixture", Secret: "fixture-app-secret"}); e != nil {
		t.Fatal(e)
	}
}
func TestPostgresSelectedOnlyAndGroupDeduplication(t *testing.T) {
	s, _ := setup(t)
	var queries atomic.Int32
	s.Fetcher = fetchFunc(func(context.Context, int64, string, string, Credential) (*http.Response, error) {
		queries.Add(1)
		return wallet("10"), nil
	})
	cs, e := s.Candidates(context.Background())
	if e != nil || len(cs) != 2 {
		t.Fatal(cs, e)
	}
	s.RunDue(context.Background())
	if queries.Load() != 0 {
		t.Fatal("unselected target queried")
	}
	site := addSite(t, s, false, nil)
	s.RunDue(context.Background())
	if queries.Load() != 1 {
		t.Fatal("scope expanded")
	}
	sites, e := s.List(context.Background())
	if e != nil || len(sites) != 1 || len(sites[0].Groups) != 2 {
		t.Fatal(sites, e)
	}
	if e = s.Remove(context.Background(), site.ID, site.Revision); e != nil {
		t.Fatal(e)
	}
	s.RunDue(context.Background())
	if queries.Load() != 1 {
		t.Fatal("removed target queried")
	}
	again := addSite(t, s, false, nil)
	if again.ID != site.ID {
		t.Fatal("history identity lost on re-add")
	}
}
func TestPostgresThresholdEpisodeAndRecoveryAreDurable(t *testing.T) {
	s, sender := setup(t)
	configure(t, s)
	value := "10"
	s.Fetcher = fetchFunc(func(context.Context, int64, string, string, Credential) (*http.Response, error) {
		return wallet(value), nil
	})
	threshold := "10"
	site := addSite(t, s, true, &threshold)
	ctx := context.Background()
	if _, e := s.Refresh(ctx, site.ID); e != nil {
		t.Fatal(e)
	}
	s.DispatchDue(ctx)
	if sender.count.Load() != 0 {
		t.Fatal("equal threshold alerted")
	}
	value = "9.99"
	for i := 0; i < 3; i++ {
		if _, e := s.Refresh(ctx, site.ID); e != nil {
			t.Fatal(e)
		}
		s.DispatchDue(ctx)
	}
	if sender.count.Load() != 1 {
		t.Fatal("duplicate low balance alert", sender.count.Load())
	}
	value = "10"
	for i := 0; i < 2; i++ {
		if _, e := s.Refresh(ctx, site.ID); e != nil {
			t.Fatal(e)
		}
	}
	current, _ := s.site(ctx, site.ID)
	if current.AlertActive {
		t.Fatal("did not recover")
	}
	value = "-0.01"
	if _, e := s.Refresh(ctx, site.ID); e != nil {
		t.Fatal(e)
	}
	s.DispatchDue(ctx)
	if sender.count.Load() != 2 {
		t.Fatal("new episode not delivered")
	}
}
func TestPostgresFailedQueryRetainsBalanceAndDoesNotRecover(t *testing.T) {
	s, _ := setup(t)
	v := "5"
	bad := false
	s.Fetcher = fetchFunc(func(context.Context, int64, string, string, Credential) (*http.Response, error) {
		if bad {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		return wallet(v), nil
	})
	threshold := "10"
	site := addSite(t, s, true, &threshold)
	ctx := context.Background()
	current, e := s.Refresh(ctx, site.ID)
	if e != nil {
		t.Fatal(e)
	}
	bad = true
	current, e = s.Refresh(ctx, site.ID)
	if e != nil || current.Amount == nil || *current.Amount != "5" || !current.AlertActive || current.Status != "unsupported" {
		t.Fatal(current, e)
	}
	var count int
	if e = s.DB.QueryRow(`SELECT count(*) FROM upstream_balance_deliveries`).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
}
func TestPostgresPauseCancelsQueuedAlertAndSecretsStayEncrypted(t *testing.T) {
	s, sender := setup(t)
	configure(t, s)
	threshold := "10"
	site := addSite(t, s, true, &threshold)
	s.Fetcher = fetchFunc(func(context.Context, int64, string, string, Credential) (*http.Response, error) {
		return wallet("1"), nil
	})
	ctx := context.Background()
	site, e := s.Refresh(ctx, site.ID)
	if e != nil {
		t.Fatal(e)
	}
	input := SiteInput{Provider: site.Provider, AccountID: site.AccountID, Name: site.Name, Enabled: false, WarningEnabled: true, Threshold: site.Threshold, RechargeURL: site.RechargeURL, Revision: site.Revision}
	if _, e = s.Update(ctx, site.ID, input); e != nil {
		t.Fatal(e)
	}
	s.DispatchDue(ctx)
	if sender.count.Load() != 0 {
		t.Fatal("paused alert sent")
	}
	var encrypted string
	if e = s.DB.QueryRow(`SELECT secret_cipher FROM upstream_balance_channel`).Scan(&encrypted); e != nil || strings.Contains(encrypted, "fixture-app-secret") {
		t.Fatal("plaintext secret")
	}
	c, e := s.Channel(ctx)
	raw, _ := json.Marshal(c)
	if e != nil || strings.Contains(string(raw), "fixture-app-secret") {
		t.Fatal("secret in public config")
	}
}
func TestPostgresRevisionFencePreventsStaleQueryWrite(t *testing.T) {
	s, _ := setup(t)
	site := addSite(t, s, false, nil)
	started, release := make(chan struct{}), make(chan struct{})
	s.Fetcher = fetchFunc(func(context.Context, int64, string, string, Credential) (*http.Response, error) {
		close(started)
		<-release
		return wallet("9"), nil
	})
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	done := make(chan error, 1)
	go func() { _, e := s.Refresh(ctx, site.ID); done <- e }()
	<-started
	_, e := s.Update(ctx, site.ID, SiteInput{Provider: site.Provider, AccountID: site.AccountID, Name: "new name", Enabled: true, RechargeURL: site.RechargeURL, Revision: site.Revision})
	if e != nil {
		t.Fatal(e)
	}
	close(release)
	if e = <-done; Code(e) != "MONITOR_CONFIG_CHANGED" {
		t.Fatal("stale write admitted", e)
	}
	current, _ := s.site(ctx, site.ID)
	if current.Amount != nil {
		t.Fatal("stale balance persisted")
	}
}
func TestPostgresExplicitTestMessageIsIdempotent(t *testing.T) {
	s, sender := setup(t)
	configure(t, s)
	id := uuid.NewString()
	for i := 0; i < 2; i++ {
		d, e := s.TestNotification(context.Background(), id, "")
		if e != nil || d.Status != "sent" || d.MessageID != "om_fixture" {
			t.Fatal(d, e)
		}
	}
	if sender.count.Load() != 1 {
		t.Fatal("explicit test was sent twice")
	}
}
func TestPostgresNewAPITokenOnlyIsNotWalletAndAccountTokenIsPrivate(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	cs, e := s.Candidates(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var c Candidate
	for _, v := range cs {
		if v.BaseURL == "https://example.com" {
			c = v
		}
	}
	site, e := s.Add(ctx, SiteInput{CandidateID: c.ID, Provider: "newapi", AccountID: 1, Name: "newapi", Enabled: true, RechargeURL: c.BaseURL})
	if e != nil {
		t.Fatal(e)
	}
	s.Fetcher = fetchFunc(func(_ context.Context, _ int64, _ string, path string, credential Credential) (*http.Response, error) {
		raw := `{"code":true,"data":{"total_available":999999,"unlimited_quota":true}}`
		if path == "/api/user/self" {
			if credential.AccessToken != "fixture-account-token" || credential.UserID != "123" {
				t.Fatal("wrong account credential")
			}
			raw = `{"success":true,"data":{"quota":2500000}}`
		} else if path == "/api/status" {
			if credential.AccessToken != "" || credential.UseAccountKey {
				t.Fatal("credential sent to public probe")
			}
			raw = `{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"USD"}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw))}, nil
	})
	current, e := s.Refresh(ctx, site.ID)
	if e != nil || current.Amount != nil || current.ErrorCode != "MONITOR_ACCOUNT_CREDENTIAL_REQUIRED" {
		t.Fatal(current, e)
	}
	site, e = s.Update(ctx, site.ID, SiteInput{Provider: "newapi", AccountID: 1, Name: "newapi", Enabled: true, RechargeURL: site.RechargeURL, AccessToken: "fixture-account-token", UserID: "123", Revision: site.Revision})
	if e != nil {
		t.Fatal(e)
	}
	current, e = s.Refresh(ctx, site.ID)
	if e != nil || current.Amount == nil || *current.Amount != "5" {
		t.Fatal(current, e)
	}
	raw, _ := json.Marshal(current)
	if strings.Contains(string(raw), "fixture-account-token") {
		t.Fatal("token leaked")
	}
}

func TestPostgresStaleDeliveryAttemptCannotOverwriteNewClaim(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	id := uuid.NewString()
	_, err := s.DB.Exec(`INSERT INTO upstream_balance_deliveries(id,episode_id,site_revision,title,amount,currency,recharge_url,is_test,status,attempts) VALUES($1,$1,0,'fixture','1','USD','https://example.com',TRUE,'sending',2)`, id)
	if err != nil {
		t.Fatal(err)
	}
	d := Delivery{ID: id, EpisodeID: id, Test: true, Attempts: 1}
	if _, err = s.finishDelivery(ctx, d, "failed", "FIXTURE_FAILURE", ""); Code(err) != "MONITOR_DELIVERY_STALE" {
		t.Fatal("old attempt overwrote new claim", err)
	}
	var status string
	var attempts int
	if err = s.DB.QueryRow(`SELECT status,attempts FROM upstream_balance_deliveries WHERE id=$1`, id).Scan(&status, &attempts); err != nil || status != "sending" || attempts != 2 {
		t.Fatal(status, attempts, err)
	}
	d.Attempts = 2
	if _, err = s.finishDelivery(ctx, d, "sent", "", "om_current"); err != nil {
		t.Fatal(err)
	}
}
func TestPostgresExpiredDeliveryStopsWithoutSending(t *testing.T) {
	s, sender := setup(t)
	ctx := context.Background()
	id := uuid.NewString()
	_, err := s.DB.Exec(`INSERT INTO upstream_balance_deliveries(id,episode_id,site_revision,title,amount,currency,recharge_url,is_test,created_at) VALUES($1,$1,0,'fixture','1','USD','https://example.com',TRUE,now()-interval '1 hour')`, id)
	if err != nil {
		t.Fatal(err)
	}
	s.DispatchDue(ctx)
	var status, code string
	if err = s.DB.QueryRow(`SELECT status,error_code FROM upstream_balance_deliveries WHERE id=$1`, id).Scan(&status, &code); err != nil || status != "failed" || code != "MONITOR_DELIVERY_EXPIRED" {
		t.Fatal(status, code, err)
	}
	if sender.count.Load() != 0 {
		t.Fatal("expired task dispatched")
	}
	s.DispatchDue(ctx)
	var events int
	if err = s.DB.QueryRow(`SELECT count(*) FROM upstream_balance_events WHERE kind='delivery'`).Scan(&events); err != nil || events != 1 {
		t.Fatal("expiry log duplicated", events, err)
	}
}

func TestPostgresDefaultsToFiveAndPreservesCustomThresholds(t *testing.T) {
	s, sender := setup(t)
	configure(t, s)
	value := "5"
	s.Fetcher = fetchFunc(func(context.Context, int64, string, string, Credential) (*http.Response, error) {
		return wallet(value), nil
	})
	ctx := context.Background()
	site := addSite(t, s, false, nil)
	if !site.WarningEnabled || site.Threshold == nil || *site.Threshold != "5" {
		t.Fatal("default threshold is not five", site)
	}
	if _, err := s.Refresh(ctx, site.ID); err != nil {
		t.Fatal(err)
	}
	s.DispatchDue(ctx)
	if sender.count.Load() != 0 {
		t.Fatal("equal default threshold alerted")
	}
	value = "4.99"
	if _, err := s.Refresh(ctx, site.ID); err != nil {
		t.Fatal(err)
	}
	s.DispatchDue(ctx)
	if sender.count.Load() != 1 {
		t.Fatal("balance below default five did not alert")
	}
	if _, err := s.DB.Exec(`UPDATE upstream_balance_sites SET warning_enabled=FALSE,threshold=0 WHERE id=$1`, site.ID); err != nil {
		t.Fatal(err)
	}
	value = "-1"
	got, err := s.Refresh(ctx, site.ID)
	if err != nil || !got.WarningEnabled || !got.AlertActive || got.Threshold == nil || *got.Threshold != "0" {
		t.Fatal("explicit zero was overwritten by default", got, err)
	}
	s.DispatchDue(ctx)
	if sender.count.Load() != 1 {
		t.Fatal("continuous low balance resent an alert")
	}
}

func TestPostgresCurrencyChangeClearsThresholdUntilReconfirmed(t *testing.T) {
	s, sender := setup(t)
	configure(t, s)
	var currency = "USD"
	s.Fetcher = fetchFunc(func(context.Context, int64, string, string, Credential) (*http.Response, error) {
		response := wallet("8")
		if currency != "USD" {
			response.Body = io.NopCloser(strings.NewReader(`{"mode":"unrestricted","balance":8,"unit":"CNY"}`))
		}
		return response, nil
	})
	threshold := "10"
	site := addSite(t, s, true, &threshold)
	ctx := context.Background()
	if _, err := s.Refresh(ctx, site.ID); err != nil {
		t.Fatal(err)
	}
	currency = "CNY"
	got, err := s.Refresh(ctx, site.ID)
	if err != nil || got.Threshold != nil || got.AlertActive || !got.WarningEnabled {
		t.Fatal("old currency threshold stayed active", got, err)
	}
	s.DispatchDue(ctx)
	if sender.count.Load() != 0 {
		t.Fatal("currency change sent an old alert")
	}
}
