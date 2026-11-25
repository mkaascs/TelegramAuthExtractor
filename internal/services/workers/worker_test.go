package workers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/require"

	"telegram-auth-extractor/internal/config"
	"telegram-auth-extractor/internal/domain"
	prom "telegram-auth-extractor/internal/infra/prometheus"
)

type mockRepo struct {
	mu          sync.Mutex
	session     *domain.Session
	getErr      error
	statuses    []domain.Status
	completed   *domain.EncryptedResult
	completeErr error
}

func (m *mockRepo) Get(_ context.Context, _ uuid.UUID) (*domain.Session, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.session, nil
}

func (m *mockRepo) Update(_ context.Context, s *domain.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses = append(m.statuses, s.Status)
	return nil
}

func (m *mockRepo) Complete(_ context.Context, s *domain.Session, result *domain.EncryptedResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.completeErr != nil {
		return m.completeErr
	}
	m.completed = result
	s.Status = domain.StatusDone
	m.statuses = append(m.statuses, domain.StatusDone)
	return nil
}

func (m *mockRepo) completedResult() *domain.EncryptedResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.completed
}

func (m *mockRepo) snapshot() []domain.Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Status, len(m.statuses))
	copy(out, m.statuses)
	return out
}

type mockDevice struct {
	mu    sync.Mutex
	calls []string

	prepareErr error
	phoneErr   error
	codeErr    error
	file       []byte
	fileErr    error
	clearErr   error
}

func (d *mockDevice) record(c string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, c)
}

func (d *mockDevice) snapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, len(d.calls))
	copy(out, d.calls)
	return out
}

func (d *mockDevice) PrepareTelegram(_ context.Context) error {
	d.record("prepare")
	return d.prepareErr
}

func (d *mockDevice) EnterPhone(_ context.Context, _ string) error {
	d.record("phone")
	return d.phoneErr
}

func (d *mockDevice) EnterCode(_ context.Context, _ string) error {
	d.record("code")
	return d.codeErr
}

func (d *mockDevice) SessionFile(_ context.Context) ([]byte, error) {
	d.record("file")
	return d.file, d.fileErr
}

func (d *mockDevice) Clear(_ context.Context) error {
	d.record("clear")
	return d.clearErr
}

type mockParser struct {
	keys []domain.DatacenterKey
	err  error
}

func (p mockParser) Parse(_ context.Context, _ []byte) ([]domain.DatacenterKey, error) {
	return p.keys, p.err
}

type mockVerifier struct {
	result *domain.ExtractedData
	err    error
}

func (v mockVerifier) Verify(_ context.Context, _ []domain.DatacenterKey) (*domain.ExtractedData, error) {
	return v.result, v.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testCfg() config.Worker {
	return config.Worker{
		QueueSize:      8,
		CodeWait:       2 * time.Second,
		CleanupTimeout: time.Second,
	}
}

func newTestMetrics() (*prom.Prometheus, *prometheus.Registry) {
	registry := prometheus.NewRegistry()
	return prom.New(registry), registry
}

func requireMetricSamples(t *testing.T, registry *prometheus.Registry, samples ...string) {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	for _, sample := range samples {
		require.Contains(t, body, sample+"\n")
	}
}

func deliverCode(t *testing.T, w *Worker, id uuid.UUID, code string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("worker never accepted the code")
		case <-tick.C:
			if err := w.SubmitCode(id, code); err == nil {
				return
			}
		}
	}
}

func cancelWhenReady(t *testing.T, w *Worker, id uuid.UUID) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("worker never became cancellable")
		case <-tick.C:
			if err := w.Cancel(id); err == nil {
				return
			}
		}
	}
}

type mockSealer struct{}

func (mockSealer) Seal(_ string, plaintext []byte) (string, error) {
	return "sealed(" + string(plaintext) + ")", nil
}

func TestWorker_Process_HappyPath(t *testing.T) {
	session := domain.NewSession("+79991234567", 0)
	id := uuid.UUID(session.ID)
	result := &domain.ExtractedData{DCID: 2, AuthKey: []byte("key"), UserID: 1153367122}

	repo := &mockRepo{session: session}
	device := &mockDevice{file: []byte("tgnet")}
	metrics, registry := newTestMetrics()
	w := NewWorker(repo, device,
		mockParser{keys: []domain.DatacenterKey{{DCID: 2}}},
		mockVerifier{result: result},
		mockSealer{}, metrics, discardLogger(), testCfg())

	done := make(chan struct{})
	go func() { w.process(context.Background(), session); close(done) }()

	deliverCode(t, w, id, "12345")
	<-done

	require.Equal(t, []domain.Status{
		domain.StatusLoggingIn,
		domain.StatusWaitingForCode,
		domain.StatusEnteringCode,
		domain.StatusExtracting,
		domain.StatusCleanup,
		domain.StatusDone,
	}, repo.snapshot())

	require.Equal(t, []string{"prepare", "phone", "code", "file", "clear"}, device.snapshot())
	require.Equal(t, domain.StatusDone, session.Status)
	require.Equal(t, &domain.EncryptedResult{
		DCID:             2,
		AuthKeyEncrypted: "sealed(key)",
		UserID:           1153367122,
	}, repo.completedResult())
	require.Nil(t, session.Error)
	requireMetricSamples(t, registry,
		`sessions_completions_total{result="done"} 1`,
		`sessions_duration_seconds_count{result="done"} 1`,
		`sessions_step_duration_seconds_count{result="done",step="waiting_for_code"} 1`,
	)
}

func TestWorker_Process_CodeTimeout(t *testing.T) {
	session := domain.NewSession("+79991234567", 0)

	cfg := testCfg()
	cfg.CodeWait = 50 * time.Millisecond

	repo := &mockRepo{session: session}
	device := &mockDevice{}
	metrics, registry := newTestMetrics()
	w := NewWorker(repo, device, mockParser{}, mockVerifier{}, mockSealer{}, metrics, discardLogger(), cfg)

	w.process(context.Background(), session)

	require.Equal(t, domain.StatusFailed, session.Status)
	require.NotNil(t, session.Error)
	require.Equal(t, domain.PublicErrMessage(domain.ErrCodeTimeout), *session.Error)

	require.Equal(t, []string{"prepare", "phone", "clear"}, device.snapshot())
	require.Equal(t, []domain.Status{
		domain.StatusLoggingIn,
		domain.StatusWaitingForCode,
		domain.StatusCleanup,
		domain.StatusFailed,
	}, repo.snapshot())
	requireMetricSamples(t, registry,
		`sessions_completions_total{result="code_timeout"} 1`,
		`sessions_duration_seconds_count{result="code_timeout"} 1`,
		`sessions_step_duration_seconds_count{result="code_timeout",step="waiting_for_code"} 1`,
	)
}

func TestWorker_Process_Cancelled(t *testing.T) {
	session := domain.NewSession("+79991234567", 0)
	id := uuid.UUID(session.ID)

	repo := &mockRepo{session: session}
	device := &mockDevice{}
	metrics, registry := newTestMetrics()
	w := NewWorker(repo, device, mockParser{}, mockVerifier{}, mockSealer{}, metrics, discardLogger(), testCfg())

	done := make(chan struct{})
	go func() { w.process(context.Background(), session); close(done) }()

	cancelWhenReady(t, w, id)
	<-done

	require.NotEqual(t, domain.StatusFailed, session.Status)
	require.NotEqual(t, domain.StatusDone, session.Status)
	require.Nil(t, session.Error)
	require.Contains(t, device.snapshot(), "clear")
	requireMetricSamples(t, registry,
		`sessions_completions_total{result="canceled"} 1`,
		`sessions_duration_seconds_count{result="canceled"} 1`,
		`sessions_step_duration_seconds_count{result="canceled",step="waiting_for_code"} 1`,
	)
}

func TestWorker_Process_DeviceError(t *testing.T) {
	session := domain.NewSession("+79991234567", 0)

	repo := &mockRepo{session: session}
	device := &mockDevice{phoneErr: errors.New("adb: no device")}
	metrics, _ := newTestMetrics()
	w := NewWorker(repo, device, mockParser{}, mockVerifier{}, mockSealer{}, metrics, discardLogger(), testCfg())

	w.process(context.Background(), session)

	require.Equal(t, domain.StatusFailed, session.Status)
	require.NotNil(t, session.Error)
	require.Equal(t, []string{"prepare", "phone", "clear"}, device.snapshot())
}

func TestWorker_Process_ParserError(t *testing.T) {
	session := domain.NewSession("+79991234567", 0)
	id := uuid.UUID(session.ID)

	repo := &mockRepo{session: session}
	device := &mockDevice{file: []byte("tgnet")}
	metrics, _ := newTestMetrics()
	w := NewWorker(repo, device,
		mockParser{err: errors.New("bad tgnet format")},
		mockVerifier{},
		mockSealer{}, metrics, discardLogger(), testCfg())

	done := make(chan struct{})
	go func() { w.process(context.Background(), session); close(done) }()

	deliverCode(t, w, id, "12345")
	<-done

	require.Equal(t, domain.StatusFailed, session.Status)
	require.NotNil(t, session.Error)
	require.Nil(t, repo.completedResult())
	require.Contains(t, device.snapshot(), "clear") // cleanup всё равно
}

func TestWorker_Process_CompleteError(t *testing.T) {
	session := domain.NewSession("+79991234567", 0)
	repo := &mockRepo{session: session, completeErr: errors.New("database unavailable")}
	metrics, registry := newTestMetrics()
	w := NewWorker(repo, &mockDevice{}, mockParser{}, mockVerifier{result: &domain.ExtractedData{}},
		mockSealer{}, metrics, discardLogger(), testCfg())

	done := make(chan struct{})
	go func() { w.process(context.Background(), session); close(done) }()

	deliverCode(t, w, uuid.UUID(session.ID), "12345")
	<-done

	require.Equal(t, domain.StatusFailed, session.Status)
	require.Nil(t, repo.completedResult())
	requireMetricSamples(t, registry,
		`sessions_completions_total{result="failed"} 1`,
		`sessions_duration_seconds_count{result="failed"} 1`,
	)
}

func TestWorker_Process_DeletedWhileQueued(t *testing.T) {
	session := domain.NewSession("+79991234567", 0)

	repo := &mockRepo{getErr: domain.ErrSessionNotFound}
	device := &mockDevice{}
	metrics, _ := newTestMetrics()
	w := NewWorker(repo, device, mockParser{}, mockVerifier{}, mockSealer{}, metrics, discardLogger(), testCfg())

	w.process(context.Background(), session)

	require.Empty(t, device.snapshot(), "по удалённой задаче устройство не трогаем")
	require.Empty(t, repo.snapshot(), "и статусы не меняем")
}

func TestWorker_SubmitCode_Idle(t *testing.T) {
	metrics, _ := newTestMetrics()
	w := NewWorker(&mockRepo{}, &mockDevice{}, mockParser{}, mockVerifier{}, mockSealer{}, metrics, discardLogger(), testCfg())
	err := w.SubmitCode(uuid.New(), "12345")
	require.ErrorIs(t, err, domain.ErrWrongState)
}

func TestWorker_Cancel_Idle(t *testing.T) {
	metrics, _ := newTestMetrics()
	w := NewWorker(&mockRepo{}, &mockDevice{}, mockParser{}, mockVerifier{}, mockSealer{}, metrics, discardLogger(), testCfg())
	err := w.Cancel(uuid.New())
	require.ErrorIs(t, err, domain.ErrSessionNotFound)
}
