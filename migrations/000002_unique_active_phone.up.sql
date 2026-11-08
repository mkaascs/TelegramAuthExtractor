CREATE UNIQUE INDEX uq_sessions_active_phone
    ON sessions(phone)
    WHERE status NOT IN ('done', 'failed');
