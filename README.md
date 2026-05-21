# ZenMind

从 **禅道（Zentao）MySQL** 同步数据到本地 **PostgreSQL**，提供 Web 控制台：数据源配置、系统用户与项目组维护、个人工作台、分析看板，以及 **可配置周期间隔** 的定时同步与手动同步。

面向**使用者**的上手说明见本文；架构、开发、API、ETL 等见 **[docs/](docs/)**。

## 功能概览

- **账号与权限**：管理员登录、系统用户管理、JWT 鉴权
- **系统配置**：禅道 MySQL 连接、禅道接口配置、自动同步周期（分钟）
- **项目组**：自定义分组与成员维护（关联已同步用户）
- **个人工作台**：我的任务、今日工时、快捷报工、个人日历聚合
- **个人集成**：日历账户 / ICS 订阅、禅道授权绑定
- **分析看板**：迭代看板、员工看板、团队健康度
- **数据明细**：任务、需求、Bug、工时、迭代等列表查询
- **同步**：YAML 表映射、水印增量/全量；定时 + 手动触发

## 使用 Docker 部署（推荐）

本机安装 **Docker**（含 Compose 插件）即可，无需单独装 PostgreSQL、Go 或 Node。

```bash
git clone <本仓库地址>
cd ZenMind
cp .env.example .env
```

编辑 `.env`，至少修改 **`JWT_SECRET`**、**`ADMIN_PASS`**。禅道 MySQL 可在启动后于 Web「系统配置」中填写。

**默认使用 [Docker Hub](https://hub.docker.com/u/techxtry) 上的预构建镜像**（命名空间默认 `techxtry`，镜像 `zenmind-backend` / `zenmind-frontend`，标签默认 `latest`，可用 **`DOCKERHUB_NAMESPACE`**、**`ZENMIND_IMAGE_TAG`** 覆盖）：

```bash
docker compose pull
docker compose up -d
```

浏览器访问 **`http://localhost:2024`**。改端口可在 `.env` 中设 `WEB_PORT`，再执行一次 `docker compose up -d`。

**从源码本地构建**（改前后端代码或调试 Dockerfile 时用）：

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

PostgreSQL / Redis / 后端默认不映射到宿主机端口，仅前端对外；首次登录可使用 `.env` 中的 **`ADMIN_USER`** / **`ADMIN_PASS`**（用户名未改时默认为 `admin`），再在 Web 端创建系统用户并分配禅道绑定、项目组等配置。

### 更新

已 `git clone` 的仓库：`git pull` 后若需新版镜像，执行 `docker compose pull && docker compose up -d`；若使用本地构建，则 `docker compose ... up -d --build`（命令同上，含 `docker-compose.build.yml`）。

自 **ZenMind** 更名后，Docker 镜像名为 `zenmind-backend` / `zenmind-frontend`，数据库默认用户/库名为 `zenmind`。若你曾用旧名 `zenboard-*` 镜像或数据卷，需重新 `pull` 新镜像；已有 Postgres 卷可继续用（连接串与 `.env` 中 `POSTGRES_*` 需与创建卷时一致），或删卷后按 `.env.example` 重建。

后端启动时会自动执行仓库内嵌的待执行数据库迁移脚本；因此升级到新版本时，**只要更新后端并重启容器 / 进程即可自动补齐 schema 变更**，无需手动逐个执行 `backend/migrations/*.sql`。

更多说明见 [docs/技术说明.md](docs/技术说明.md)。