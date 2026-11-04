UPDATE sessions SET status = 'pending' WHERE status = 'queued';

ALTER TABLE sessions DROP CONSTRAINT sessions_status_check;

ALTER TABLE sessions ADD CONSTRAINT sessions_status_check
    CHECK ( status IN ('pending', 'logging_in', 'waiting_for_code',
                       'entering_code', 'extracting', 'cleanup', 'done', 'failed') );
