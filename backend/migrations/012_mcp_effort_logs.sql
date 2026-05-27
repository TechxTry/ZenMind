-- MCP 报工记录表：本地落库 + 禅道写回状态追踪
CREATE TABLE IF NOT EXISTS mcp_effort_logs (
    id                 BIGSERIAL PRIMARY KEY,
    client_request_id  TEXT        NOT NULL,
    actor_username     TEXT        NOT NULL,
    object_type        TEXT        NOT NULL DEFAULT 'task',
    object_id          BIGINT      NOT NULL,
    work_date          DATE        NOT NULL,
    consumed           NUMERIC(10,2) NOT NULL,
    work               TEXT        NOT NULL,
    status             TEXT        NOT NULL DEFAULT 'pending',
    zentao_effort_id   BIGINT,
    zentao_mode        TEXT,
    error_message      TEXT,
    retry_count        INT         NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT mcp_effort_logs_request_id_uq UNIQUE (client_request_id)
);

CREATE INDEX IF NOT EXISTS idx_mcp_effort_logs_actor ON mcp_effort_logs(actor_username);
CREATE INDEX IF NOT EXISTS idx_mcp_effort_logs_status ON mcp_effort_logs(status);
CREATE INDEX IF NOT EXISTS idx_mcp_effort_logs_object ON mcp_effort_logs(object_type, object_id);
