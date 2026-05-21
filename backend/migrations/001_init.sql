-- ============================================================
-- ZenMind — Phase 1 DDL
-- PostgreSQL 15+
-- ============================================================

-- 人员基表 (源: zt_user)
CREATE TABLE IF NOT EXISTS local_users (
  id         BIGINT PRIMARY KEY,
  account    VARCHAR(60)  UNIQUE NOT NULL,
  realname   VARCHAR(60),
  role       VARCHAR(30),
  deleted    BOOLEAN      NOT NULL DEFAULT FALSE,
  raw_data   JSONB,
  synced_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_local_users_deleted ON local_users(deleted);

-- 自定义小组 (本地原生)
CREATE TABLE IF NOT EXISTS project_groups (
  id          SERIAL PRIMARY KEY,
  name        VARCHAR(120) UNIQUE NOT NULL,
  description TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 组成员桥接表 (联合PK)
CREATE TABLE IF NOT EXISTS group_members (
  group_id  INT         NOT NULL REFERENCES project_groups(id) ON DELETE CASCADE,
  account   VARCHAR(60) NOT NULL REFERENCES local_users(account) ON DELETE CASCADE,
  PRIMARY KEY (group_id, account)
);

-- 迭代 (源: zt_project / zt_execution)
CREATE TABLE IF NOT EXISTS local_executions (
  id         BIGINT PRIMARY KEY,
  name       TEXT,
  status     VARCHAR(30),
  begin_date DATE,
  end_date   DATE,
  deleted    BOOLEAN     NOT NULL DEFAULT FALSE,
  raw_data   JSONB,
  synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_local_executions_status    ON local_executions(status);
CREATE INDEX IF NOT EXISTS idx_local_executions_date_range ON local_executions(begin_date, end_date);

-- 任务 (源: zt_task)
CREATE TABLE IF NOT EXISTS local_tasks (
  id                BIGINT PRIMARY KEY,
  name              TEXT,
  type              VARCHAR(30),
  status            VARCHAR(30),
  assigned_to       VARCHAR(60),
  finished_by       VARCHAR(60),
  estimate          FLOAT,
  consumed          FLOAT,
  execution_id      BIGINT,
  story_id          BIGINT,
  last_edited_date  TIMESTAMPTZ,
  deleted           BOOLEAN     NOT NULL DEFAULT FALSE,
  raw_data          JSONB,
  synced_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_local_tasks_status           ON local_tasks(status);
CREATE INDEX IF NOT EXISTS idx_local_tasks_assigned_to      ON local_tasks(assigned_to);
CREATE INDEX IF NOT EXISTS idx_local_tasks_last_edited_date ON local_tasks(last_edited_date);
CREATE INDEX IF NOT EXISTS idx_local_tasks_execution_id     ON local_tasks(execution_id);

-- 需求 (源: zt_story)
CREATE TABLE IF NOT EXISTS local_stories (
  id                BIGINT PRIMARY KEY,
  title             TEXT,
  status            VARCHAR(30),
  assigned_to       VARCHAR(60),
  estimate          FLOAT,
  product_id        BIGINT,
  last_edited_date  TIMESTAMPTZ,
  deleted           BOOLEAN     NOT NULL DEFAULT FALSE,
  raw_data          JSONB,
  synced_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_local_stories_status           ON local_stories(status);
CREATE INDEX IF NOT EXISTS idx_local_stories_assigned_to      ON local_stories(assigned_to);
CREATE INDEX IF NOT EXISTS idx_local_stories_last_edited_date ON local_stories(last_edited_date);

-- 缺陷 (源: zt_bug)
CREATE TABLE IF NOT EXISTS local_bugs (
  id                BIGINT PRIMARY KEY,
  title             TEXT,
  severity          INT,
  status            VARCHAR(30),
  assigned_to       VARCHAR(60),
  resolved_by       VARCHAR(60),
  resolution        VARCHAR(60),
  execution_id      BIGINT,
  story_id          BIGINT,
  task_id           BIGINT,
  last_edited_date  TIMESTAMPTZ,
  deleted           BOOLEAN     NOT NULL DEFAULT FALSE,
  raw_data          JSONB,
  synced_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_local_bugs_severity        ON local_bugs(severity);
CREATE INDEX IF NOT EXISTS idx_local_bugs_status          ON local_bugs(status);
CREATE INDEX IF NOT EXISTS idx_local_bugs_assigned_to     ON local_bugs(assigned_to);
CREATE INDEX IF NOT EXISTS idx_local_bugs_resolved_by     ON local_bugs(resolved_by);
CREATE INDEX IF NOT EXISTS idx_local_bugs_last_edited_date ON local_bugs(last_edited_date);

-- 报工 (源: zt_effort)
CREATE TABLE IF NOT EXISTS local_efforts (
  id          BIGINT PRIMARY KEY,
  account     VARCHAR(60),
  work_date   DATE,
  consumed    FLOAT,
  work        TEXT,
  object_type VARCHAR(30),
  object_id   BIGINT,
  deleted     BOOLEAN     NOT NULL DEFAULT FALSE,
  raw_data    JSONB,
  synced_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_local_efforts_account_date ON local_efforts(account, work_date);
CREATE INDEX IF NOT EXISTS idx_local_efforts_object       ON local_efforts(object_type, object_id);

-- ETL 水位线表
CREATE TABLE IF NOT EXISTS sync_watermarks (
  table_name VARCHAR(60) PRIMARY KEY,
  watermark  TIMESTAMPTZ,
  last_count BIGINT      DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 初始化水位线记录（各表从零开始全量同步一次）
INSERT INTO sync_watermarks (table_name, watermark) VALUES
  ('local_users',      '1970-01-01 00:00:00+00'),
  ('local_tasks',      '1970-01-01 00:00:00+00'),
  ('local_stories',    '1970-01-01 00:00:00+00'),
  ('local_bugs',       '1970-01-01 00:00:00+00'),
  ('local_efforts',    '1970-01-01 00:00:00+00'),
  ('local_executions', '1970-01-01 00:00:00+00')
ON CONFLICT (table_name) DO NOTHING;

-- 应用级可配置项（首次启动时 Go 也会 CREATE IF NOT EXISTS；本脚本供全新库 docker init 或手动 psql）
CREATE TABLE IF NOT EXISTS app_settings (
  setting_key TEXT PRIMARY KEY,
  value       TEXT NOT NULL,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO app_settings (setting_key, value) VALUES ('sync_interval_minutes', '15')
ON CONFLICT (setting_key) DO NOTHING;
