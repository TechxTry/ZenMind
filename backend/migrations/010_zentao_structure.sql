-- ============================================================
-- ZenMind — Zentao structure (Program/Project/ProductLine/Product)
-- PostgreSQL 15+
-- ============================================================

-- 项目集 / 计划 (源: zt_project type='program' 等，按 etl_tables.yaml 配置)
CREATE TABLE IF NOT EXISTS local_programs (
  id         BIGINT PRIMARY KEY,
  name       TEXT,
  status     VARCHAR(30),
  parent_id  BIGINT,
  path       TEXT,
  grade      INT,
  begin_date DATE,
  end_date   DATE,
  deleted    BOOLEAN     NOT NULL DEFAULT FALSE,
  raw_data   JSONB,
  synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- 兼容：老版本表可能缺列（CREATE TABLE IF NOT EXISTS 不会补列）
ALTER TABLE local_programs ADD COLUMN IF NOT EXISTS parent_id BIGINT;
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'local_programs'
      AND column_name = 'parent_id'
  ) THEN
    EXECUTE 'CREATE INDEX IF NOT EXISTS idx_local_programs_parent_id ON local_programs(parent_id)';
  END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_local_programs_status    ON local_programs(status);

-- 项目 (源: zt_project type='project' 等，按 etl_tables.yaml 配置)
CREATE TABLE IF NOT EXISTS local_projects (
  id         BIGINT PRIMARY KEY,
  name       TEXT,
  status     VARCHAR(30),
  parent_id  BIGINT,         -- 通常指向 program
  path       TEXT,
  grade      INT,
  begin_date DATE,
  end_date   DATE,
  deleted    BOOLEAN     NOT NULL DEFAULT FALSE,
  raw_data   JSONB,
  synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE local_projects ADD COLUMN IF NOT EXISTS parent_id BIGINT;
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'local_projects'
      AND column_name = 'parent_id'
  ) THEN
    EXECUTE 'CREATE INDEX IF NOT EXISTS idx_local_projects_parent_id ON local_projects(parent_id)';
  END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_local_projects_status    ON local_projects(status);

-- 给迭代表补充 parent/type，便于“项目→迭代”过滤
-- 注意：部分 PG 驱动/执行器在单次 Exec 多语句场景下对 ALTER+CREATE INDEX 的可见性存在差异，
-- 这里拆成多段，并在建索引前显式检查列存在，保证幂等与可启动性。
ALTER TABLE local_executions ADD COLUMN IF NOT EXISTS parent_id BIGINT;
ALTER TABLE local_executions ADD COLUMN IF NOT EXISTS type VARCHAR(30);
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'local_executions'
      AND column_name = 'parent_id'
  ) THEN
    EXECUTE 'CREATE INDEX IF NOT EXISTS idx_local_executions_parent_id ON local_executions(parent_id)';
  END IF;
END $$;

-- 产品线（企业版可能存在 zt_line，若你们实例无该表也不影响：可不启用同步）
CREATE TABLE IF NOT EXISTS local_product_lines (
  id         BIGINT PRIMARY KEY,
  name       TEXT,
  parent_id  BIGINT,
  path       TEXT,
  grade      INT,
  deleted    BOOLEAN     NOT NULL DEFAULT FALSE,
  raw_data   JSONB,
  synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE local_product_lines ADD COLUMN IF NOT EXISTS parent_id BIGINT;
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'local_product_lines'
      AND column_name = 'parent_id'
  ) THEN
    EXECUTE 'CREATE INDEX IF NOT EXISTS idx_local_product_lines_parent_id ON local_product_lines(parent_id)';
  END IF;
END $$;

-- 产品（源: zt_product）
CREATE TABLE IF NOT EXISTS local_products (
  id         BIGINT PRIMARY KEY,
  name       TEXT,
  code       VARCHAR(60),
  status     VARCHAR(30),
  line_id    BIGINT,
  deleted    BOOLEAN     NOT NULL DEFAULT FALSE,
  raw_data   JSONB,
  synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- 兼容：老版本表可能缺列（CREATE TABLE IF NOT EXISTS 不会补列）
ALTER TABLE local_products ADD COLUMN IF NOT EXISTS line_id BIGINT;
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'local_products'
      AND column_name = 'line_id'
  ) THEN
    EXECUTE 'CREATE INDEX IF NOT EXISTS idx_local_products_line_id ON local_products(line_id)';
  END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_local_products_status  ON local_products(status);

-- 维表水位线（维表默认全量覆盖，水位线仅用于状态展示）
INSERT INTO sync_watermarks (table_name, watermark) VALUES
  ('local_programs',      '1970-01-01 00:00:00+00'),
  ('local_projects',      '1970-01-01 00:00:00+00'),
  ('local_product_lines', '1970-01-01 00:00:00+00'),
  ('local_products',      '1970-01-01 00:00:00+00')
ON CONFLICT (table_name) DO NOTHING;

