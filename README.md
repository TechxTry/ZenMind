# ZenMind

从 **禅道（Zentao）MySQL** 同步数据到本地 **PostgreSQL**，提供 Web 控制台：数据源与业务配置、系统用户与项目组、个人工作台、分析看板，以及 **可配置周期** 的定时同步与手动同步。普通用户还可在界面生成密钥，把任务 / 需求 / Bug / 报工接到 **Cursor 等 MCP 客户端**。

面向**使用者**的上手说明见本文；架构、开发、API、ETL 等见 **[docs/](docs/)**。

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Docker Hub](https://img.shields.io/docker/v/techxtry/zenmind-backend?label=backend&logo=docker)](https://hub.docker.com/r/techxtry/zenmind-backend)
[![Docker Hub](https://img.shields.io/docker/v/techxtry/zenmind-frontend?label=frontend&logo=docker)](https://hub.docker.com/r/techxtry/zenmind-frontend)

## 功能概览

- **账号与权限**：管理员登录、系统用户管理、角色与数据范围（本人 / 小组 / 全量）、审计日志
- **系统配置**：禅道 MySQL 连接、禅道接口、日标准工时、自动同步周期（分钟）
- **项目组**：自定义分组与成员维护（关联已同步用户）
- **个人工作台**：我的任务 / Bug、今日工时、快捷报工、个人日历聚合；任务 / 需求 / Bug 可在工作台内创建与维护
- **个人集成**：日历账户 / ICS 订阅、禅道授权绑定
- **MCP**：个人访问密钥；在 Cursor 等客户端查询与操作任务、需求、Bug、报工
- **分析看板**：迭代看板、员工看板、团队健康度
- **数据明细**：任务、需求、Bug、工时、迭代、项目等列表查询
- **同步**：YAML 表映射、水印增量 / 全量、每日报工回刷；定时 + 手动触发

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

编辑 `.env`，至少修改 **`JWT_SECRET`**、**`ADMIN_PASS`**。禅道 MySQL 可在启动后于 Web「数据同步」中填写。

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

PostgreSQL / Redis / 后端仅在容器网络内访问，对外只暴露前端端口。

数据库表结构由**后端镜像**在启动时自动迁移；升级时拉取新后端镜像并重启即可，无需手动执行 SQL。

### 首次配置

1. 使用 `.env` 中的 **`ADMIN_USER`** / **`ADMIN_PASS`** 登录（用户名未改时默认为 `admin`）。
2. 在 **数据同步** 填写禅道 MySQL，测试连接后触发一次同步。
3. 在 **业务配置** 填写禅道 Web 地址（个人工作台写操作、MCP 写操作需要）。
4. 在 **账号管理** 创建系统用户，并 **1:1 绑定**禅道账号；按需维护 **小组**。
5. 普通用户登录后：绑定禅道授权 → 使用个人工作台报工；需要 AI 接入时到 **MCP 访问** 生成密钥。

权限与绑定说明见 [docs/账号与权限.md](docs/账号与权限.md)、[docs/禅道绑定与个人口径.md](docs/禅道绑定与个人口径.md)。

### 从源码本地构建

仅在修改前后端代码或调试 Dockerfile 时需要整仓源码：

```bash
git clone https://github.com/TechxTry/ZenMind.git
cd ZenMind
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

## MCP（Cursor 等客户端）

登录后打开 **个人工作台 → MCP 访问**，生成个人密钥（只展示一次）。在支持 MCP 的客户端配置中写入（将密钥替换为真实值）：

```json
{
  "mcpServers": {
    "zenmind": {
      "url": "http://localhost:2024/api/mcp",
      "headers": {
        "Authorization": "Bearer zmcp_xxx_your_token"
      }
    }
  }
}
```

若部署在其他主机或端口，把 `url` 改成实际访问地址（与浏览器打开 ZenMind 的 origin 相同，路径为 `/api/mcp`）。修改配置后请重启客户端或重新加载 MCP。

当前可查询产品 / 项目 / 人员 / 计划、我的任务 / 需求 / Bug / 报工 / 执行，以及创建、更新、删除任务、需求、Bug 与报工。写操作走禅道接口，需已绑定禅道账号并完成禅道授权。

## 更新

| 部署方式 | 操作 |
|----------|------|
| 仅编排目录（`curl` 下载） | 重新下载最新的 `docker-compose.yml`（若 `.env.example` 有变可对照合并），然后 `docker compose pull && docker compose up -d` |
| 已 `git clone` | `git pull` 后执行 `docker compose pull && docker compose up -d` |
| 本地构建 | `git pull` 后执行 `docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build` |

自 **ZenMind** 更名后，镜像名为 `zenmind-backend` / `zenmind-frontend`，数据库默认用户/库名为 `zenmind`。若曾使用旧名 `zenboard-*` 镜像，请 `pull` 新镜像；已有 Postgres 数据卷可继续使用（`.env` 中 `POSTGRES_*` 须与建卷时一致）。

更多说明见 [docs/技术说明.md](docs/技术说明.md)。命令行报工（`zmcli`）亦见该文档。

## License

[Apache License 2.0](LICENSE)
