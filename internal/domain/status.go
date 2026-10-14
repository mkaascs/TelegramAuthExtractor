package domain

type Status string

const (
	StatusPending        Status = "pending"
	StatusLoggingIn      Status = "logging_in"
	StatusWaitingForCode Status = "waiting_for_code"
	StatusExtracting     Status = "extracting"
	StatusVerifying      Status = "verifying"
	StatusCleanup        Status = "cleanup"
	StatusDone           Status = "done"
	StatusFailed         Status = "failed"
)
