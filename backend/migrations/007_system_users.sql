-- ============================================================
-- ZenMind — Phase 2: System Users / Bindings / Audit Logs
-- PostgreSQL 15+
-- ============================================================

-- 系统登录用户（与 local_users 区分：local_users 是禅道镜像人员表）
CREATE TABLE IF NOT EXISTS system_users (
  id               BIGSERIAL PRIMARY KEY,
  username         VARCHAR(60) UNIQUE NOT NULL,
  display_name     TEXT,
  password_hash    TEXT NOT NULL,
  role             VARCHAR(30) NOT NULL DEFAULT 'user', -- super_admin/admin/user
  data_scope       VARCHAR(30) NOT NULL DEFAULT 'SELF', -- SELF/GROUP/ALL
  default_group_id INT NULL REFERENCES project_groups(id) ON DELETE SET NULL,
  disabled         BOOLEAN NOT NULL DEFAULT FALSE,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_system_users_disabled ON system_users(disabled);
CREATE INDEX IF NOT EXISTS idx_system_users_role ON system_users(role);

-- 系统用户 ↔ 禅道账号绑定（1:1，禁止复用）
CREATE TABLE IF NOT EXISTS zentao_bindings (
  id              BIGSERIAL PRIMARY KEY,
  system_user_id  BIGINT NOT NULL UNIQUE REFERENCES system_users(id) ON DELETE CASCADE,
  zentao_account  VARCHAR(60) NOT NULL UNIQUE REFERENCES local_users(account) ON DELETE RESTRICT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_zentao_bindings_account ON zentao_bindings(zentao_account);

-- 审计日志（关键操作留痕）
CREATE TABLE IF NOT EXISTS audit_logs (
  id              BIGSERIAL PRIMARY KEY,
  actor_user_id   BIGINT NULL REFERENCES system_users(id) ON DELETE SET NULL,
  actor_username  TEXT,
  action          TEXT NOT NULL,
  target_type     TEXT,
  target_id       TEXT,
  metadata        JSONB,
  ip              TEXT,
  ua              TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_user_id ON audit_logs(actor_user_id);

