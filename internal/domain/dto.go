package domain

type CreateSessionResult struct {
	Session       *Session
	Token         string
	DecryptionKey string
	Created       bool
}
