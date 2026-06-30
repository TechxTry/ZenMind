-- ETL / 报工回刷 执行记录（自动、手动、定时）
CREATE TABLE IF NOT EXISTS sync_run_logs (
    id              BIGSERIAL PRIMARY KEY,
    job_type        TEXT        NOT NULL,
    trigger_source  TEXT        NOT NULL,
    status          TEXT        NOT NULL,
    message         TEXT,
    actor_username  TEXT,
    metadata        JSONB       NOT NULL DEFAULT '{}',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ,
    duration_ms     BIGINT
);

CREATE INDEX IF NOT EXISTS idx_sync_run_logs_started_at ON sync_run_logs (started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sync_run_logs_job_type ON sync_run_logs (job_type, started_at DESC);
