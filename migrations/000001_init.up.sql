CREATE TABLE sessions(
    id uuid DEFAULT uuidv7() PRIMARY KEY,
    phone TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
                     CHECK ( status IN ('pending', 'logging_in', 'waiting_for_code',
                                        'entering_code', 'extracting', 'cleanup', 'done', 'failed') ),
    error TEXT,
    code_attempts INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE results(
    id uuid DEFAULT uuidv7() PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    dc_id INTEGER NOT NULL,
    auth_key BYTEA NOT NULL,
    user_id BIGINT NOT NULL
);

CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

CREATE INDEX idx_results_session_id ON results(session_id);