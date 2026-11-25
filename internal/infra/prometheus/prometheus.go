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
		Help: "Total number of completed session processing attempts by result",
	}, []string{"result"})

	sessionsStepDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sessions_step_duration_seconds",
		Help:    "Duration in seconds of observed session steps by result",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 200},
	}, []string{"step", "result"})

	sessionsDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sessions_duration_seconds",
		Help:    "Duration in seconds of session processing attempts by result",
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

func (p *Prometheus) RecordCompletion(result string, elapsed time.Duration) {
	p.sessionsCompletionsTotal.WithLabelValues(result).Inc()
	p.sessionsDuration.WithLabelValues(result).Observe(elapsed.Seconds())
}

func (p *Prometheus) ObserveSessionStep(step domain.Status, result string, elapsed time.Duration) {
	p.sessionsStepDuration.WithLabelValues(string(step), result).Observe(elapsed.Seconds())
}
