package prometheus

import (
	"telegram-auth-extractor/internal/domain"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Prometheus struct {
	sessionsDuration         *prometheus.HistogramVec
	sessionsStepDuration     *prometheus.HistogramVec
	sessionsCompletionsTotal *prometheus.CounterVec
	registerer               prometheus.Registerer
}

func New(reg prometheus.Registerer) *Prometheus {
	sessionsCompletionsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sessions_completions_total",
		Help: "Total number of finished sessions by result",
	}, []string{"result"})

	sessionsStepDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sessions_step_duration_seconds",
		Help:    "The total time in seconds taken to complete one step of a session",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 200},
	}, []string{"step"})

	sessionsDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sessions_duration_seconds",
		Help:    "The total time in seconds taken to complete a session",
		Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
	}, []string{"result"})

	reg.MustRegister(sessionsCompletionsTotal, sessionsStepDuration, sessionsDuration)

	return &Prometheus{
		sessionsCompletionsTotal: sessionsCompletionsTotal,
		sessionsStepDuration:     sessionsStepDuration,
		sessionsDuration:         sessionsDuration,
		registerer:               reg,
	}
}

func (p *Prometheus) RegisterQueueDepth(depth func() int) {
	queueDepth := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "queue_depth",
		Help: "Number of sessions waiting in the worker queue",
	}, func() float64 { return float64(depth()) })

	p.registerer.MustRegister(queueDepth)
}

func (p *Prometheus) CompletionDone() {
	p.sessionsCompletionsTotal.WithLabelValues("done").Inc()
}

func (p *Prometheus) CompletionScreenTimeout() {
	p.sessionsCompletionsTotal.WithLabelValues("timeout").Inc()
}

func (p *Prometheus) CompletionScreenCancel() {
	p.sessionsCompletionsTotal.WithLabelValues("canceled").Inc()
}

func (p *Prometheus) CompletionFailed() {
	p.sessionsCompletionsTotal.WithLabelValues("failed").Inc()
}

func (p *Prometheus) ObserveSessionStep(step domain.Status, elapsed time.Duration) {
	p.sessionsStepDuration.WithLabelValues(string(step)).Observe(elapsed.Seconds())
}

func (p *Prometheus) ObserveSessionDone(elapsed time.Duration) {
	p.sessionsDuration.WithLabelValues("done").Observe(elapsed.Seconds())
}

func (p *Prometheus) ObserveSessionFailed(elapsed time.Duration) {
	p.sessionsDuration.WithLabelValues("failed").Observe(elapsed.Seconds())
}
