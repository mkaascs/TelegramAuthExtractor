package domain

type SessionOutcome string

const (
	OutcomeDone        SessionOutcome = "done"
	OutcomeFailed      SessionOutcome = "failed"
	OutcomeCanceled    SessionOutcome = "canceled"
	OutcomeCodeTimeout SessionOutcome = "code_timeout"
)
