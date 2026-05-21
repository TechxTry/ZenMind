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

本机只需安装 **Docker**（含 Compose 插件）。**不必**安装 PostgreSQL、Go 或 Node，也**不必**拉取整仓源码来编译——默认从 [Docker Hub](https://hub.docker.com/u/techxtry) 拉取预构建镜像（`zenmind-backend` / `zenmind-frontend`，命名空间默认 `techxtry`，标签默认 `latest`；可用环境变量 **`DOCKERHUB_NAMESPACE`**、**`ZENMIND_IMAGE_TAG`** 覆盖）。

### 最快上手（仅编排文件）

在任意空目录执行（从 GitHub 下载 `docker-compose.yml` 与 `.env.example` 即可）：

```bash
mkdir zenmind && cd zenmind

curl -fsSLO https://raw.githubusercontent.com/TechxTry/ZenMind/main/docker-compose.yml
curl -fsSLO https://raw.githubusercontent.com/TechxTry/ZenMind/main/.env.example
cp .env.example .env
```

编辑 `.env`，至少修改 **`JWT_SECRET`**、**`ADMIN_PASS`**。禅道 MySQL 可在启动后于 Web「系统配置」中填写。

```bash
docker compose pull
docker compose up -d
```

浏览器访问 **`http://localhost:2024`**。改对外端口：在 `.env` 中设置 `WEB_PORT`，再执行 `docker compose up -d`。

### 使用 Git 克隆（便于跟进编排变更）

若希望用 `git pull` 同步 `docker-compose.yml` 等文件的更新，可克隆仓库后同样只需根目录下的编排与环境变量：

```bash
git clone https://github.com/TechxTry/ZenMind.git
cd ZenMind
cp .env.example .env
# 编辑 .env 后：
docker compose pull && docker compose up -d
```

首次登录使用 `.env` 中的 **`ADMIN_USER`** / **`ADMIN_PASS`**（用户名未改时默认为 `admin`），再在 Web 端创建系统用户并配置禅道绑定、项目组等。PostgreSQL / Redis / 后端仅在容器网络内访问，对外只暴露前端端口。

数据库表结构由**后端镜像**在启动时自动迁移；升级时拉取新后端镜像并重启即可，无需手动执行 SQL。

### 从源码本地构建

仅在修改前后端代码或调试 Dockerfile 时需要整仓源码：

```bash
git clone https://github.com/TechxTry/ZenMind.git
cd ZenMind
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

### 更新

| 部署方式 | 操作 |
|----------|------|
| 仅编排目录（`curl` 下载） | 重新下载最新的 `docker-compose.yml`（若 `.env.example` 有变可对照合并），然后 `docker compose pull && docker compose up -d` |
| 已 `git clone` | `git pull` 后执行 `docker compose pull && docker compose up -d` |
| 本地构建 | `git pull` 后执行 `docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build` |

自 **ZenMind** 更名后，镜像名为 `zenmind-backend` / `zenmind-frontend`，数据库默认用户/库名为 `zenmind`。若曾使用旧名 `zenboard-*` 镜像，请 `pull` 新镜像；已有 Postgres 数据卷可继续使用（`.env` 中 `POSTGRES_*` 须与建卷时一致）。

更多说明见 [docs/技术说明.md](docs/技术说明.md)。
