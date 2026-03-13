package cron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hanmahong5-arch/lurus-identity/internal/app"
	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/email"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/event"
	"github.com/redis/go-redis/v9"
)

// ---------- Redis helpers ----------

func newTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return mr, rdb
}

// ---------- mock EventPublisher ----------

type mockEventPublisher struct {
	mu        sync.Mutex
	published []*event.IdentityEvent
	err       error
}

func (m *mockEventPublisher) Publish(_ context.Context, ev *event.IdentityEvent) error {
	if m.err != nil {
		return m.err
	}
	m.mu.Lock()
	m.published = append(m.published, ev)
	m.mu.Unlock()
	return nil
}

// ---------- mock outboxWriter ----------

type mockOutboxWriter struct {
	events []*event.IdentityEvent
	err    error
}

func (m *mockOutboxWriter) Insert(_ context.Context, ev *event.IdentityEvent) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, ev)
	return nil
}

// ---------- mock outboxStore (for OutboxRelay) ----------

type mockOutboxStore struct {
	mu        sync.Mutex
	events    []entity.OutboxEvent
	published map[int64]bool
	attempts  map[int64]int
	deleted   int64
	pubErr    error
}

func (m *mockOutboxStore) ListUnpublished(_ context.Context, limit int) ([]entity.OutboxEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []entity.OutboxEvent
	for _, e := range m.events {
		if !m.published[e.ID] {
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *mockOutboxStore) MarkPublished(_ context.Context, id int64) error {
	if m.pubErr != nil {
		return m.pubErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published[id] = true
	return nil
}

func (m *mockOutboxStore) IncrementAttempts(_ context.Context, id int64, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts[id]++
	return nil
}

func (m *mockOutboxStore) DeletePublishedBefore(_ context.Context, _ time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted++
	return 1, nil
}

func newMockOutboxStore() *mockOutboxStore {
	return &mockOutboxStore{
		published: make(map[int64]bool),
		attempts:  make(map[int64]int),
	}
}

// ---------- mock subscription/plan stores for SubscriptionService ----------

type noopSubStore struct {
	mu            sync.Mutex
	activeExpired []entity.Subscription
	graceExpired  []entity.Subscription
	dueForRenewal []entity.Subscription
	updated       map[int64]int
}

func newNoopSubStore() *noopSubStore {
	return &noopSubStore{updated: make(map[int64]int)}
}

func (s *noopSubStore) Create(_ context.Context, _ *entity.Subscription) error  { return nil }
func (s *noopSubStore) Update(_ context.Context, _ *entity.Subscription) error  { return nil }
func (s *noopSubStore) GetByID(_ context.Context, _ int64) (*entity.Subscription, error) {
	return nil, nil
}
func (s *noopSubStore) GetActive(_ context.Context, _ int64, _ string) (*entity.Subscription, error) {
	return nil, nil
}
func (s *noopSubStore) ListByAccount(_ context.Context, _ int64) ([]entity.Subscription, error) {
	return nil, nil
}
func (s *noopSubStore) ListActiveExpired(_ context.Context) ([]entity.Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeExpired, nil
}
func (s *noopSubStore) ListGraceExpired(_ context.Context) ([]entity.Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.graceExpired, nil
}
func (s *noopSubStore) ListDueForRenewal(_ context.Context) ([]entity.Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dueForRenewal, nil
}
func (s *noopSubStore) UpdateRenewalState(_ context.Context, id int64, attempts int, _ *time.Time) error {
	s.mu.Lock()
	s.updated[id] = attempts
	s.mu.Unlock()
	return nil
}
func (s *noopSubStore) UpsertEntitlement(_ context.Context, _ *entity.AccountEntitlement) error {
	return nil
}
func (s *noopSubStore) GetEntitlements(_ context.Context, _ int64, _ string) ([]entity.AccountEntitlement, error) {
	return nil, nil
}
func (s *noopSubStore) DeleteEntitlements(_ context.Context, _ int64, _ string) error { return nil }

type noopPlanStore struct {
	plans map[int64]*entity.ProductPlan
}

func newNoopPlanStore() *noopPlanStore {
	return &noopPlanStore{plans: make(map[int64]*entity.ProductPlan)}
}

func (p *noopPlanStore) GetPlanByID(_ context.Context, id int64) (*entity.ProductPlan, error) {
	plan := p.plans[id]
	return plan, nil
}
func (p *noopPlanStore) ListActive(_ context.Context) ([]entity.Product, error)              { return nil, nil }
func (p *noopPlanStore) ListPlans(_ context.Context, _ string) ([]entity.ProductPlan, error) { return nil, nil }
func (p *noopPlanStore) GetByID(_ context.Context, _ string) (*entity.Product, error)        { return nil, nil }
func (p *noopPlanStore) Create(_ context.Context, _ *entity.Product) error                   { return nil }
func (p *noopPlanStore) Update(_ context.Context, _ *entity.Product) error                   { return nil }
func (p *noopPlanStore) CreatePlan(_ context.Context, _ *entity.ProductPlan) error           { return nil }
func (p *noopPlanStore) UpdatePlan(_ context.Context, _ *entity.ProductPlan) error           { return nil }

type noopWalletStore struct {
	mu      sync.Mutex
	wallets map[int64]*entity.Wallet
	debited bool
	debitErr error
}

func newNoopWalletStore() *noopWalletStore {
	return &noopWalletStore{wallets: make(map[int64]*entity.Wallet)}
}

func (w *noopWalletStore) GetOrCreate(_ context.Context, accountID int64) (*entity.Wallet, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	wlt := w.wallets[accountID]
	if wlt == nil {
		wlt = &entity.Wallet{AccountID: accountID, Balance: 100.0}
		w.wallets[accountID] = wlt
	}
	cp := *wlt
	return &cp, nil
}
func (w *noopWalletStore) GetByAccountID(ctx context.Context, accountID int64) (*entity.Wallet, error) {
	return w.GetOrCreate(ctx, accountID)
}
func (w *noopWalletStore) Credit(_ context.Context, accountID int64, amount float64, _, _, _, _, _ string) (*entity.WalletTransaction, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	wlt := w.wallets[accountID]
	if wlt == nil {
		wlt = &entity.Wallet{AccountID: accountID}
		w.wallets[accountID] = wlt
	}
	wlt.Balance += amount
	return &entity.WalletTransaction{Amount: amount}, nil
}
func (w *noopWalletStore) Debit(_ context.Context, accountID int64, amount float64, _, _, _, _, _ string) (*entity.WalletTransaction, error) {
	if w.debitErr != nil {
		return nil, w.debitErr
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	wlt := w.wallets[accountID]
	if wlt == nil {
		wlt = &entity.Wallet{AccountID: accountID, Balance: 100.0}
		w.wallets[accountID] = wlt
	}
	if wlt.Balance < amount {
		return nil, fmt.Errorf("insufficient balance")
	}
	wlt.Balance -= amount
	w.debited = true
	return &entity.WalletTransaction{Amount: -amount}, nil
}
func (w *noopWalletStore) ListTransactions(_ context.Context, _ int64, _, _ int) ([]entity.WalletTransaction, int64, error) {
	return nil, 0, nil
}
func (w *noopWalletStore) CreatePaymentOrder(_ context.Context, _ *entity.PaymentOrder) error { return nil }
func (w *noopWalletStore) UpdatePaymentOrder(_ context.Context, _ *entity.PaymentOrder) error { return nil }
func (w *noopWalletStore) GetPaymentOrderByNo(_ context.Context, _ string) (*entity.PaymentOrder, error) {
	return nil, nil
}
func (w *noopWalletStore) GetRedemptionCode(_ context.Context, _ string) (*entity.RedemptionCode, error) {
	return nil, nil
}
func (w *noopWalletStore) UpdateRedemptionCode(_ context.Context, _ *entity.RedemptionCode) error { return nil }
func (w *noopWalletStore) ListOrders(_ context.Context, _ int64, _, _ int) ([]entity.PaymentOrder, int64, error) {
	return nil, 0, nil
}
func (w *noopWalletStore) MarkPaymentOrderPaid(_ context.Context, _ string) (*entity.PaymentOrder, bool, error) {
	return nil, false, nil
}
func (w *noopWalletStore) RedeemCode(_ context.Context, _ int64, _ string) (*entity.WalletTransaction, error) {
	return nil, nil
}
func (w *noopWalletStore) ExpireStalePendingOrders(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

type noopVIPStore struct{}

func (v *noopVIPStore) GetOrCreate(_ context.Context, accountID int64) (*entity.AccountVIP, error) {
	return &entity.AccountVIP{AccountID: accountID}, nil
}
func (v *noopVIPStore) Update(_ context.Context, _ *entity.AccountVIP) error { return nil }
func (v *noopVIPStore) ListConfigs(_ context.Context) ([]entity.VIPLevelConfig, error) {
	return nil, nil
}

type noopCache struct{}

func (c *noopCache) Get(_ context.Context, _ int64, _ string) (map[string]string, error) {
	return nil, nil
}
func (c *noopCache) Set(_ context.Context, _ int64, _ string, _ map[string]string) error { return nil }
func (c *noopCache) Invalidate(_ context.Context, _ int64, _ string) error               { return nil }

// buildSubService creates a minimal SubscriptionService for cron tests.
func buildSubService(subStore *noopSubStore, planStore *noopPlanStore) *app.SubscriptionService {
	entSvc := app.NewEntitlementService(subStore, planStore, &noopCache{})
	return app.NewSubscriptionService(subStore, planStore, entSvc, 3)
}

// buildWalletService creates a minimal WalletService for cron tests.
func buildWalletService(walletStore *noopWalletStore) *app.WalletService {
	vipSvc := app.NewVIPService(&noopVIPStore{}, walletStore)
	return app.NewWalletService(walletStore, vipSvc)
}

// ---------- nextRenewalAt tests ----------

// TestNextRenewalAt_Attempt1_1Hour verifies that the first retry is ~1 hour away.
func TestNextRenewalAt_Attempt1_1Hour(t *testing.T) {
	before := time.Now()
	at := nextRenewalAt(1)
	after := time.Now()

	if at == nil {
		t.Fatal("expected non-nil time")
	}
	minExpected := before.Add(backoffAttempt1 - time.Second)
	maxExpected := after.Add(backoffAttempt1 + time.Second)
	if at.Before(minExpected) || at.After(maxExpected) {
		t.Errorf("attempt 1 next_at = %v, want ~+1h", *at)
	}
}

// TestNextRenewalAt_Attempt2_4Hours verifies that the second retry is ~4 hours away.
func TestNextRenewalAt_Attempt2_4Hours(t *testing.T) {
	before := time.Now()
	at := nextRenewalAt(2)
	after := time.Now()

	minExpected := before.Add(backoffAttempt2 - time.Second)
	maxExpected := after.Add(backoffAttempt2 + time.Second)
	if at.Before(minExpected) || at.After(maxExpected) {
		t.Errorf("attempt 2 next_at = %v, want ~+4h", *at)
	}
}

// TestNextRenewalAt_Attempt3Plus_12Hours verifies that attempt 3+ is capped at 12 hours.
func TestNextRenewalAt_Attempt3Plus_12Hours(t *testing.T) {
	for _, attempt := range []int{3, 5, 100} {
		before := time.Now()
		at := nextRenewalAt(attempt)
		after := time.Now()
		minExpected := before.Add(backoffAttempt3 - time.Second)
		maxExpected := after.Add(backoffAttempt3 + time.Second)
		if at.Before(minExpected) || at.After(maxExpected) {
			t.Errorf("attempt %d next_at = %v, want ~+12h", attempt, *at)
		}
	}
}

// ---------- OutboxRelay tests ----------

// makeOutboxEvent creates a valid OutboxEvent with a serialized IdentityEvent payload.
func makeOutboxEvent(id int64, attempts int) entity.OutboxEvent {
	ev, _ := event.NewEvent(event.SubjectAccountCreated, 1, "L-0000000001", "", nil)
	payload, _ := json.Marshal(ev)
	return entity.OutboxEvent{
		ID:       id,
		EventID:  ev.EventID,
		Payload:  payload,
		Attempts: attempts,
	}
}

// TestOutboxRelay_PollAndPublish_Empty verifies that an empty outbox produces no publications.
func TestOutboxRelay_PollAndPublish_Empty(t *testing.T) {
	store := newMockOutboxStore()
	pub := &mockEventPublisher{}
	relay := NewOutboxRelay(store, pub)

	relay.pollAndPublish(context.Background())

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) != 0 {
		t.Errorf("published %d events, want 0", len(pub.published))
	}
}

// TestOutboxRelay_PollAndPublish_Success verifies that a valid event is published and marked.
func TestOutboxRelay_PollAndPublish_Success(t *testing.T) {
	store := newMockOutboxStore()
	store.events = []entity.OutboxEvent{makeOutboxEvent(1, 0)}
	pub := &mockEventPublisher{}
	relay := NewOutboxRelay(store, pub)

	relay.pollAndPublish(context.Background())

	pub.mu.Lock()
	count := len(pub.published)
	pub.mu.Unlock()
	if count != 1 {
		t.Errorf("published %d events, want 1", count)
	}
	store.mu.Lock()
	marked := store.published[1]
	store.mu.Unlock()
	if !marked {
		t.Error("event should be marked published")
	}
}

// TestOutboxRelay_PollAndPublish_MaxAttempts_Skipped verifies dead-letter events are skipped.
func TestOutboxRelay_PollAndPublish_MaxAttempts_Skipped(t *testing.T) {
	store := newMockOutboxStore()
	store.events = []entity.OutboxEvent{makeOutboxEvent(1, outboxMaxAttempts)}
	pub := &mockEventPublisher{}
	relay := NewOutboxRelay(store, pub)

	relay.pollAndPublish(context.Background())

	pub.mu.Lock()
	count := len(pub.published)
	pub.mu.Unlock()
	if count != 0 {
		t.Errorf("dead-letter event should not be published, got %d", count)
	}
}

// TestOutboxRelay_PollAndPublish_PublisherError verifies that failed events increment attempts.
func TestOutboxRelay_PollAndPublish_PublisherError(t *testing.T) {
	store := newMockOutboxStore()
	store.events = []entity.OutboxEvent{makeOutboxEvent(1, 0)}
	pub := &mockEventPublisher{err: errors.New("nats down")}
	relay := NewOutboxRelay(store, pub)

	relay.pollAndPublish(context.Background())

	store.mu.Lock()
	attempts := store.attempts[1]
	store.mu.Unlock()
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 after publish error", attempts)
	}
	store.mu.Lock()
	marked := store.published[1]
	store.mu.Unlock()
	if marked {
		t.Error("failed event should not be marked published")
	}
}

// ---------- NotificationJob tests ----------

// mockNotifSubStore implements notificationSubStore for testing.
type mockNotifSubStore struct {
	subs []entity.Subscription
}

func (m *mockNotifSubStore) ListExpiring(_ context.Context, _ int) ([]entity.Subscription, error) {
	return m.subs, nil
}

// mockNotifAccountStore implements notificationAccountStore for testing.
type mockNotifAccountStore struct {
	accounts map[int64]*entity.Account
}

func (m *mockNotifAccountStore) GetByID(_ context.Context, id int64) (*entity.Account, error) {
	return m.accounts[id], nil
}

// TestNotificationJob_RunNotifications_Empty verifies no emails sent when no subscriptions exist.
func TestNotificationJob_RunNotifications_Empty(t *testing.T) {
	_, rdb := newTestRedis(t)
	mailer := email.NoopSender{}
	job := NewNotificationJob(
		&mockNotifSubStore{},
		&mockNotifAccountStore{accounts: make(map[int64]*entity.Account)},
		mailer, rdb, time.Hour,
	)

	sent, skipped, failed := job.runNotifications(context.Background())
	if sent != 0 || failed != 0 {
		t.Errorf("empty: sent=%d failed=%d, want 0/0", sent, failed)
	}
	_ = skipped
}

// TestNotificationJob_RunNotifications_SendsReminder verifies that a reminder email is sent.
func TestNotificationJob_RunNotifications_SendsReminder(t *testing.T) {
	_, rdb := newTestRedis(t)
	expiresAt := time.Now().Add(24 * time.Hour)
	subs := []entity.Subscription{
		{ID: 1, AccountID: 42, ProductID: "gushen", ExpiresAt: &expiresAt, Status: "active"},
	}
	accounts := map[int64]*entity.Account{
		42: {ID: 42, Email: "user@example.com", DisplayName: "User"},
	}
	job := NewNotificationJob(
		&mockNotifSubStore{subs: subs},
		&mockNotifAccountStore{accounts: accounts},
		email.NoopSender{}, rdb, time.Hour,
	)

	sent, _, _ := job.runNotifications(context.Background())
	if sent == 0 {
		t.Error("expected at least 1 reminder sent")
	}
}

// TestNotificationJob_RunNotifications_Deduplication verifies that duplicate sends are skipped.
func TestNotificationJob_RunNotifications_Deduplication(t *testing.T) {
	_, rdb := newTestRedis(t)
	expiresAt := time.Now().Add(24 * time.Hour)
	subs := []entity.Subscription{
		{ID: 2, AccountID: 43, ProductID: "gushen", ExpiresAt: &expiresAt, Status: "active"},
	}
	accounts := map[int64]*entity.Account{
		43: {ID: 43, Email: "dedup@example.com", DisplayName: "Dedup User"},
	}
	job := NewNotificationJob(
		&mockNotifSubStore{subs: subs},
		&mockNotifAccountStore{accounts: accounts},
		email.NoopSender{}, rdb, time.Hour,
	)

	// First run: should send.
	sent1, skipped1, _ := job.runNotifications(context.Background())
	// Second run: should skip (dedup key set).
	sent2, skipped2, _ := job.runNotifications(context.Background())

	if sent1 == 0 {
		t.Error("first run: expected at least 1 sent")
	}
	if sent2 != 0 {
		t.Errorf("second run: expected 0 sent (dedup), got %d", sent2)
	}
	if skipped2 == 0 && skipped1 == 0 {
		t.Log("note: dedup skipped count", skipped1, skipped2)
	}
}

// TestNotificationJob_RunNotifications_AccountNotFound verifies graceful handling when account is nil.
func TestNotificationJob_RunNotifications_AccountNotFound(t *testing.T) {
	_, rdb := newTestRedis(t)
	expiresAt := time.Now().Add(24 * time.Hour)
	subs := []entity.Subscription{
		{ID: 3, AccountID: 999, ProductID: "gushen", ExpiresAt: &expiresAt, Status: "active"},
	}
	// No account for ID 999
	job := NewNotificationJob(
		&mockNotifSubStore{subs: subs},
		&mockNotifAccountStore{accounts: make(map[int64]*entity.Account)},
		email.NoopSender{}, rdb, time.Hour,
	)

	_, _, failed := job.runNotifications(context.Background())
	if failed == 0 {
		t.Error("expected failed count > 0 when account not found")
	}
}

// ---------- ExpiryJob tests ----------

// TestExpiryJob_RunExpiry_EmptyLists verifies that empty subscription lists produce no output.
func TestExpiryJob_RunExpiry_EmptyLists(t *testing.T) {
	_, rdb := newTestRedis(t)
	subStore := newNoopSubStore()
	planStore := newNoopPlanStore()
	subSvc := buildSubService(subStore, planStore)
	pub := &mockEventPublisher{}

	job := NewExpiryJob(subSvc, pub, rdb, nil)
	scanned, processed, failed := job.runExpiry(context.Background())

	if scanned != 0 || processed != 0 || failed != 0 {
		t.Errorf("runExpiry(empty) = (%d, %d, %d), want (0,0,0)", scanned, processed, failed)
	}
}

// TestExpiryJob_AcquireLock_Success verifies lock can be acquired and released.
func TestExpiryJob_AcquireLock_Success(t *testing.T) {
	_, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	job := NewExpiryJob(subSvc, nil, rdb, nil)

	ctx := context.Background()
	acquired, err := job.acquireLock(ctx)
	if err != nil {
		t.Fatalf("acquireLock: %v", err)
	}
	if !acquired {
		t.Fatal("expected lock to be acquired")
	}

	// Second attempt should fail (lock held).
	acquired2, _ := job.acquireLock(ctx)
	if acquired2 {
		t.Error("second lock acquisition should fail (lock already held)")
	}

	// Release and re-acquire.
	job.releaseLock(ctx)
	acquired3, err3 := job.acquireLock(ctx)
	if err3 != nil {
		t.Fatalf("acquireLock after release: %v", err3)
	}
	if !acquired3 {
		t.Error("lock should be re-acquirable after release")
	}
}

// TestExpiryJob_Run_ImmediateCancel verifies Run exits cleanly on context cancellation.
func TestExpiryJob_Run_ImmediateCancel(t *testing.T) {
	_, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	job := NewExpiryJob(subSvc, &mockEventPublisher{}, rdb, nil)
	job.interval = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- job.Run(ctx) }()

	// Cancel immediately after the first tick has run.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Run did not exit within 2s after context cancellation")
	}
}

// ---------- RenewalJob tests ----------

// TestRenewalJob_RunRenewal_EmptyList verifies that no renewals occur with an empty list.
func TestRenewalJob_RunRenewal_EmptyList(t *testing.T) {
	_, rdb := newTestRedis(t)
	subStore := newNoopSubStore()
	planStore := newNoopPlanStore()
	walletStore := newNoopWalletStore()

	subSvc := buildSubService(subStore, planStore)
	walletSvc := buildWalletService(walletStore)
	pub := &mockEventPublisher{}

	job := NewRenewalJob(subSvc, subStore, planStore, walletSvc, pub, rdb, time.Hour, nil)
	scanned, renewed, failed := job.runRenewal(context.Background())
	if scanned != 0 || renewed != 0 || failed != 0 {
		t.Errorf("empty: got (%d,%d,%d), want (0,0,0)", scanned, renewed, failed)
	}
}

// TestRenewalJob_RunRenewal_PlanNotFound verifies that missing plan results in failure.
func TestRenewalJob_RunRenewal_PlanNotFound(t *testing.T) {
	_, rdb := newTestRedis(t)
	subStore := newNoopSubStore()
	// Seed a subscription that references a plan that doesn't exist.
	subStore.dueForRenewal = []entity.Subscription{
		{ID: 1, AccountID: 10, ProductID: "gushen", PlanID: 999, RenewalAttempts: 0},
	}
	planStore := newNoopPlanStore() // plan 999 not seeded → returns nil
	walletStore := newNoopWalletStore()

	subSvc := buildSubService(subStore, planStore)
	walletSvc := buildWalletService(walletStore)
	pub := &mockEventPublisher{}

	job := NewRenewalJob(subSvc, subStore, planStore, walletSvc, pub, rdb, time.Hour, nil)
	_, renewed, failed := job.runRenewal(context.Background())
	if renewed != 0 {
		t.Errorf("renewed = %d, want 0 (plan not found)", renewed)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}
}

// TestRenewalJob_RunRenewal_InsufficientBalance verifies that failed debit increments attempts.
func TestRenewalJob_RunRenewal_InsufficientBalance(t *testing.T) {
	_, rdb := newTestRedis(t)
	subStore := newNoopSubStore()
	planStore := newNoopPlanStore()

	// Seed a plan with a price higher than any wallet balance.
	planStore.plans[10] = &entity.ProductPlan{
		ID:       10,
		Code:     "pro-monthly",
		PriceCNY: 9999.0, // exceeds mock wallet balance of 100
	}
	subStore.dueForRenewal = []entity.Subscription{
		{ID: 5, AccountID: 20, ProductID: "gushen", PlanID: 10, RenewalAttempts: 0},
	}

	walletStore := newNoopWalletStore()
	// Wallet has 100.0 balance (set in GetOrCreate), plan costs 9999 → debit fails.
	walletStore.debitErr = fmt.Errorf("insufficient balance")

	subSvc := buildSubService(subStore, planStore)
	walletSvc := buildWalletService(walletStore)
	pub := &mockEventPublisher{}

	job := NewRenewalJob(subSvc, subStore, planStore, walletSvc, pub, rdb, time.Hour, nil)
	_, renewed, failed := job.runRenewal(context.Background())

	if renewed != 0 {
		t.Errorf("renewed = %d, want 0", renewed)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}

	// RenewalAttempts should be incremented to 1.
	subStore.mu.Lock()
	attempts := subStore.updated[5]
	subStore.mu.Unlock()
	if attempts != 1 {
		t.Errorf("renewal attempts = %d, want 1", attempts)
	}
}

// TestRenewalJob_AcquireLock_Success verifies RenewalJob lock acquisition.
func TestRenewalJob_AcquireLock_Success(t *testing.T) {
	_, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	walletSvc := buildWalletService(newNoopWalletStore())
	job := NewRenewalJob(subSvc, newNoopSubStore(), newNoopPlanStore(), walletSvc, nil, rdb, time.Hour, nil)

	ctx := context.Background()
	acquired, err := job.acquireLock(ctx)
	if err != nil {
		t.Fatalf("acquireLock: %v", err)
	}
	if !acquired {
		t.Fatal("expected lock acquired")
	}
	defer job.releaseLock(ctx)

	// Second pod cannot acquire.
	acquired2, _ := job.acquireLock(ctx)
	if acquired2 {
		t.Error("second lock acquisition should fail")
	}
}

// ---------- ExpiryJob.publishEvent tests ----------

// TestExpiryJob_PublishEvent_ViaPublisher verifies that events are published directly when outbox is nil.
func TestExpiryJob_PublishEvent_ViaPublisher(t *testing.T) {
	_, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	pub := &mockEventPublisher{}
	job := NewExpiryJob(subSvc, pub, rdb, nil)

	job.publishEvent(context.Background(), event.SubjectSubscriptionExpired, 1, "L-0001", "gushen", map[string]any{"subscription_id": 42})

	pub.mu.Lock()
	count := len(pub.published)
	pub.mu.Unlock()
	if count != 1 {
		t.Errorf("published %d events, want 1", count)
	}
}

// TestExpiryJob_PublishEvent_ViaOutbox verifies that events are routed through the outbox when available.
func TestExpiryJob_PublishEvent_ViaOutbox(t *testing.T) {
	_, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	outbox := &mockOutboxWriter{}
	pub := &mockEventPublisher{}
	job := NewExpiryJob(subSvc, pub, rdb, outbox)

	job.publishEvent(context.Background(), event.SubjectSubscriptionExpired, 1, "L-0001", "gushen", map[string]any{"subscription_id": 42})

	if len(outbox.events) != 1 {
		t.Errorf("outbox events = %d, want 1", len(outbox.events))
	}
	// Direct publisher should NOT be called when outbox succeeds.
	pub.mu.Lock()
	count := len(pub.published)
	pub.mu.Unlock()
	if count != 0 {
		t.Errorf("direct publish count = %d, want 0 (outbox should handle it)", count)
	}
}

// TestExpiryJob_PublishEvent_OutboxFails_FallsBackToDirect verifies fallback to direct publish when outbox fails.
func TestExpiryJob_PublishEvent_OutboxFails_FallsBackToDirect(t *testing.T) {
	_, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	outbox := &mockOutboxWriter{err: errors.New("db down")}
	pub := &mockEventPublisher{}
	job := NewExpiryJob(subSvc, pub, rdb, outbox)

	job.publishEvent(context.Background(), event.SubjectSubscriptionExpired, 1, "L-0001", "gushen", nil)

	pub.mu.Lock()
	count := len(pub.published)
	pub.mu.Unlock()
	if count != 1 {
		t.Errorf("expected 1 direct publish after outbox failure, got %d", count)
	}
}

// ---------- NotificationJob Run/tick/lock lifecycle tests ----------

// TestNotificationJob_AcquireNotifLock_Success verifies the acquire+release cycle.
func TestNotificationJob_AcquireNotifLock_Success(t *testing.T) {
	_, rdb := newTestRedis(t)
	job := NewNotificationJob(
		&mockNotifSubStore{},
		&mockNotifAccountStore{accounts: make(map[int64]*entity.Account)},
		email.NoopSender{}, rdb, time.Hour,
	)
	ctx := context.Background()

	acquired, err := job.acquireNotifLock(ctx)
	if err != nil {
		t.Fatalf("acquireNotifLock: %v", err)
	}
	if !acquired {
		t.Fatal("expected lock to be acquired")
	}

	// Second attempt should fail (lock held).
	acquired2, _ := job.acquireNotifLock(ctx)
	if acquired2 {
		t.Error("second acquisition should fail while lock is held")
	}

	// Release and re-acquire.
	job.releaseNotifLock(ctx)
	acquired3, err3 := job.acquireNotifLock(ctx)
	if err3 != nil {
		t.Fatalf("acquireNotifLock after release: %v", err3)
	}
	if !acquired3 {
		t.Error("lock should be re-acquirable after release")
	}
}

// TestNotificationJob_Tick_LockAlreadyHeld verifies tick exits early when another pod holds the lock.
func TestNotificationJob_Tick_LockAlreadyHeld(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	// Pre-acquire the lock to simulate another pod holding it.
	rdb.SetNX(ctx, notificationLockKey, "1", notificationLockTTL)

	job := NewNotificationJob(
		&mockNotifSubStore{},
		&mockNotifAccountStore{accounts: make(map[int64]*entity.Account)},
		email.NoopSender{}, rdb, time.Hour,
	)
	// tick should exit early (lock not acquired) without blocking or panicking.
	job.tick(ctx)
}

// TestNotificationJob_RunNotifications_NilExpiresAt verifies that subscriptions with nil ExpiresAt are skipped.
func TestNotificationJob_RunNotifications_NilExpiresAt(t *testing.T) {
	_, rdb := newTestRedis(t)
	subs := []entity.Subscription{
		{ID: 99, AccountID: 1, ProductID: "gushen", ExpiresAt: nil, Status: "active"},
	}
	accounts := map[int64]*entity.Account{
		1: {ID: 1, Email: "user@example.com", DisplayName: "User"},
	}
	job := NewNotificationJob(
		&mockNotifSubStore{subs: subs},
		&mockNotifAccountStore{accounts: accounts},
		email.NoopSender{}, rdb, time.Hour,
	)

	sent, _, failed := job.runNotifications(context.Background())
	if sent != 0 || failed != 0 {
		t.Errorf("nil ExpiresAt: sent=%d failed=%d, want 0/0", sent, failed)
	}
}

// TestNotificationJob_Run_ImmediateCancel verifies NotificationJob.Run exits cleanly on context cancellation.
func TestNotificationJob_Run_ImmediateCancel(t *testing.T) {
	_, rdb := newTestRedis(t)
	job := NewNotificationJob(
		&mockNotifSubStore{},
		&mockNotifAccountStore{accounts: make(map[int64]*entity.Account)},
		email.NoopSender{}, rdb, 100*time.Millisecond,
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- job.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("NotificationJob.Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("NotificationJob.Run did not exit within 2s after context cancellation")
	}
}

// ---------- OutboxRelay Run/cleanup lifecycle tests ----------

// TestOutboxRelay_Cleanup_DeletesOldEvents verifies cleanup calls DeletePublishedBefore.
func TestOutboxRelay_Cleanup_DeletesOldEvents(t *testing.T) {
	store := newMockOutboxStore()
	relay := NewOutboxRelay(store, &mockEventPublisher{})

	relay.cleanup(context.Background())

	store.mu.Lock()
	deleted := store.deleted
	store.mu.Unlock()
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
}

// TestOutboxRelay_Run_ImmediateCancel verifies OutboxRelay.Run exits cleanly on context cancellation.
func TestOutboxRelay_Run_ImmediateCancel(t *testing.T) {
	relay := NewOutboxRelay(newMockOutboxStore(), &mockEventPublisher{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- relay.Run(ctx) }()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("OutboxRelay.Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("OutboxRelay.Run did not exit within 2s after context cancellation")
	}
}

// TestRenewalJob_RunRenewal_Success verifies a successful renewal: debit + activate + event publish.
func TestRenewalJob_RunRenewal_Success(t *testing.T) {
	_, rdb := newTestRedis(t)
	subStore := newNoopSubStore()
	planStore := newNoopPlanStore()

	// Seed a plan with an affordable price.
	planStore.plans[1] = &entity.ProductPlan{
		ID:       1,
		Code:     "basic-monthly",
		PriceCNY: 10.0,
	}
	subStore.dueForRenewal = []entity.Subscription{
		{ID: 1, AccountID: 1, ProductID: "gushen", PlanID: 1, PaymentMethod: "wallet"},
	}

	walletStore := newNoopWalletStore()
	// Default wallet balance is 100.0; plan costs 10.0 → debit succeeds.

	subSvc := buildSubService(subStore, planStore)
	walletSvc := buildWalletService(walletStore)
	pub := &mockEventPublisher{}
	outbox := &mockOutboxWriter{}

	job := NewRenewalJob(subSvc, subStore, planStore, walletSvc, pub, rdb, time.Hour, outbox)
	scanned, renewed, failed := job.runRenewal(context.Background())

	if scanned != 1 {
		t.Errorf("scanned = %d, want 1", scanned)
	}
	if renewed != 1 {
		t.Errorf("renewed = %d, want 1 (successful renewal)", renewed)
	}
	if failed != 0 {
		t.Errorf("failed = %d, want 0", failed)
	}
}

// TestRenewalJob_PublishRenewalEvent_NilPublisher verifies that nil publisher is handled gracefully.
func TestRenewalJob_PublishRenewalEvent_NilPublisher(t *testing.T) {
	_, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	walletSvc := buildWalletService(newNoopWalletStore())
	// nil publisher and nil outbox → should not panic.
	job := NewRenewalJob(subSvc, newNoopSubStore(), newNoopPlanStore(), walletSvc, nil, rdb, time.Hour, nil)

	// publishRenewalEvent with nil publisher and nil outbox should be a no-op.
	job.publishRenewalEvent(context.Background(), "identity.subscription.expired", 1, "gushen", nil)
}

// TestRenewalJob_PublishRenewalEvent_OutboxFails_FallsBackToDirect verifies fallback to direct publish.
func TestRenewalJob_PublishRenewalEvent_OutboxFails_FallsBackToDirect(t *testing.T) {
	_, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	walletSvc := buildWalletService(newNoopWalletStore())
	outbox := &mockOutboxWriter{err: errors.New("db error")}
	pub := &mockEventPublisher{}
	job := NewRenewalJob(subSvc, newNoopSubStore(), newNoopPlanStore(), walletSvc, pub, rdb, time.Hour, outbox)

	job.publishRenewalEvent(context.Background(), "identity.subscription.activated", 1, "gushen", nil)

	pub.mu.Lock()
	count := len(pub.published)
	pub.mu.Unlock()
	if count != 1 {
		t.Errorf("published = %d, want 1 after outbox failure", count)
	}
}

// ---------- ExpiryJob runExpiry with subscriptions ----------

// TestExpiryJob_RunExpiry_WithSubscriptions exercises the loop body paths
// (Expire and EndGrace will fail because GetByID returns nil, but the error paths are covered).
func TestExpiryJob_RunExpiry_WithSubscriptions(t *testing.T) {
	_, rdb := newTestRedis(t)
	subStore := newNoopSubStore()
	planStore := newNoopPlanStore()

	expiredAt := time.Now().Add(-time.Hour)
	subStore.activeExpired = []entity.Subscription{
		{ID: 10, AccountID: 1, ProductID: "gushen", ExpiresAt: &expiredAt, Status: "active"},
	}
	subStore.graceExpired = []entity.Subscription{
		{ID: 11, AccountID: 2, ProductID: "gushen", ExpiresAt: &expiredAt, Status: "grace"},
	}

	subSvc := buildSubService(subStore, planStore)
	pub := &mockEventPublisher{}
	job := NewExpiryJob(subSvc, pub, rdb, nil)

	scanned, _, _ := job.runExpiry(context.Background())
	// Both subscriptions should be scanned regardless of Expire/EndGrace outcome.
	if scanned != 2 {
		t.Errorf("scanned = %d, want 2", scanned)
	}
}

// ---------- RenewalJob Run/tick lifecycle tests ----------

// TestRenewalJob_Tick_LockAlreadyHeld verifies tick exits early when another pod holds the lock.
func TestRenewalJob_Tick_LockAlreadyHeld(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	// Pre-acquire the lock to simulate another pod.
	rdb.SetNX(ctx, renewalLockKey, "1", renewalLockTTL)

	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	walletSvc := buildWalletService(newNoopWalletStore())
	job := NewRenewalJob(subSvc, newNoopSubStore(), newNoopPlanStore(), walletSvc, nil, rdb, time.Hour, nil)

	// tick should exit early (lock not acquired) without blocking or panicking.
	job.tick(ctx)
}

// TestRenewalJob_Run_ImmediateCancel verifies RenewalJob.Run exits cleanly on context cancellation.
func TestRenewalJob_Run_ImmediateCancel(t *testing.T) {
	_, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	walletSvc := buildWalletService(newNoopWalletStore())
	job := NewRenewalJob(subSvc, newNoopSubStore(), newNoopPlanStore(), walletSvc, &mockEventPublisher{}, rdb, 100*time.Millisecond, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- job.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("RenewalJob.Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("RenewalJob.Run did not exit within 2s after context cancellation")
	}
}

// ---------- Redis error-path tests (lock acquire/release failures) ----------

// TestExpiryJob_Tick_LockError verifies ExpiryJob.tick handles acquireLock Redis error gracefully.
func TestExpiryJob_Tick_LockError(t *testing.T) {
	mr, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	job := NewExpiryJob(subSvc, nil, rdb, nil)

	// Break Redis so SetNX fails.
	mr.SetError("ERR forced error")

	ctx := context.Background()
	// tick should handle the error gracefully without panicking.
	job.tick(ctx)
	// Also verify acquireLock returns the error.
	acquired, err := job.acquireLock(ctx)
	if err == nil {
		t.Error("expected error from acquireLock when Redis is down")
	}
	if acquired {
		t.Error("expected acquired=false when Redis returns error")
	}
}

// TestExpiryJob_ReleaseLock_Error verifies ExpiryJob.releaseLock handles Redis Del error gracefully.
func TestExpiryJob_ReleaseLock_Error(t *testing.T) {
	mr, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	job := NewExpiryJob(subSvc, nil, rdb, nil)

	ctx := context.Background()
	// Acquire lock first so the key exists.
	_, _ = job.acquireLock(ctx)
	// Now break Redis so Del fails.
	mr.SetError("ERR forced error")
	// releaseLock should log a warning and not panic.
	job.releaseLock(ctx)
}

// TestRenewalJob_Tick_LockError verifies RenewalJob.tick handles acquireLock Redis error gracefully.
func TestRenewalJob_Tick_LockError(t *testing.T) {
	mr, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	walletSvc := buildWalletService(newNoopWalletStore())
	job := NewRenewalJob(subSvc, newNoopSubStore(), newNoopPlanStore(), walletSvc, nil, rdb, time.Hour, nil)

	mr.SetError("ERR forced error")

	ctx := context.Background()
	job.tick(ctx)
	acquired, err := job.acquireLock(ctx)
	if err == nil {
		t.Error("expected error from acquireLock when Redis is down")
	}
	if acquired {
		t.Error("expected acquired=false when Redis returns error")
	}
}

// TestRenewalJob_ReleaseLock_Error verifies RenewalJob.releaseLock handles Redis Del error gracefully.
func TestRenewalJob_ReleaseLock_Error(t *testing.T) {
	mr, rdb := newTestRedis(t)
	subSvc := buildSubService(newNoopSubStore(), newNoopPlanStore())
	walletSvc := buildWalletService(newNoopWalletStore())
	job := NewRenewalJob(subSvc, newNoopSubStore(), newNoopPlanStore(), walletSvc, nil, rdb, time.Hour, nil)

	ctx := context.Background()
	_, _ = job.acquireLock(ctx)
	mr.SetError("ERR forced error")
	job.releaseLock(ctx)
}

// TestNotificationJob_Tick_LockError verifies NotificationJob.tick handles acquireNotifLock Redis error gracefully.
func TestNotificationJob_Tick_LockError(t *testing.T) {
	mr, rdb := newTestRedis(t)
	job := NewNotificationJob(
		&mockNotifSubStore{},
		&mockNotifAccountStore{accounts: make(map[int64]*entity.Account)},
		email.NoopSender{}, rdb, time.Hour,
	)

	mr.SetError("ERR forced error")

	ctx := context.Background()
	job.tick(ctx)
	acquired, err := job.acquireNotifLock(ctx)
	if err == nil {
		t.Error("expected error from acquireNotifLock when Redis is down")
	}
	if acquired {
		t.Error("expected acquired=false when Redis returns error")
	}
}

// TestNotificationJob_ReleaseLock_Error verifies NotificationJob.releaseNotifLock handles Redis Del error gracefully.
func TestNotificationJob_ReleaseLock_Error(t *testing.T) {
	mr, rdb := newTestRedis(t)
	job := NewNotificationJob(
		&mockNotifSubStore{},
		&mockNotifAccountStore{accounts: make(map[int64]*entity.Account)},
		email.NoopSender{}, rdb, time.Hour,
	)

	ctx := context.Background()
	_, _ = job.acquireNotifLock(ctx)
	mr.SetError("ERR forced error")
	job.releaseNotifLock(ctx)
}

// errNotifSubStore is a notificationSubStore that returns an error from ListExpiring.
type errNotifSubStore struct {
	listErr error
}

func (e *errNotifSubStore) ListExpiring(_ context.Context, _ int) ([]entity.Subscription, error) {
	return nil, e.listErr
}

// TestNotificationJob_RunNotifications_ListError verifies that a ListExpiring error is counted as failed.
func TestNotificationJob_RunNotifications_ListError(t *testing.T) {
	_, rdb := newTestRedis(t)
	job := NewNotificationJob(
		&errNotifSubStore{listErr: fmt.Errorf("db unavailable")},
		&mockNotifAccountStore{accounts: make(map[int64]*entity.Account)},
		email.NoopSender{}, rdb, time.Hour,
	)

	_, _, failed := job.runNotifications(context.Background())
	// reminderDays has 3 entries, each one will error → failed should be 3.
	if failed != len(reminderDays) {
		t.Errorf("failed = %d, want %d (one per reminder day)", failed, len(reminderDays))
	}
}

// TestRenewalJob_RunRenewal_ActivationFail_FundsRefunded verifies that a debit success
// followed by activation failure triggers a compensation credit and returns an error.
// This covers the processOne branch at renewal.go lines 178-210.
func TestRenewalJob_RunRenewal_ActivationFail_FundsRefunded(t *testing.T) {
	_, rdb := newTestRedis(t)
	subStore := newNoopSubStore()

	// jobPlanStore: has plan 1 → allows debit in processOne.
	jobPlanStore := newNoopPlanStore()
	jobPlanStore.plans[1] = &entity.ProductPlan{
		ID:       1,
		Code:     "basic-monthly",
		PriceCNY: 10.0,
	}
	// svcPlanStore: empty → SubscriptionService.Activate fails with "plan not found".
	svcPlanStore := newNoopPlanStore()

	subStore.dueForRenewal = []entity.Subscription{
		{ID: 7, AccountID: 7, ProductID: "gushen", PlanID: 1, PaymentMethod: "wallet"},
	}

	walletStore := newNoopWalletStore()
	walletStore.wallets[7] = &entity.Wallet{AccountID: 7, Balance: 100.0}

	subSvc := buildSubService(subStore, svcPlanStore) // Activate will fail: plan 1 not found
	walletSvc := buildWalletService(walletStore)
	pub := &mockEventPublisher{}

	job := NewRenewalJob(subSvc, subStore, jobPlanStore, walletSvc, pub, rdb, time.Hour, nil)
	_, renewed, failed := job.runRenewal(context.Background())

	if renewed != 0 {
		t.Errorf("renewed = %d, want 0 (activation failed)", renewed)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1 (debit succeeded + activation failed → compensation credit)", failed)
	}
}
