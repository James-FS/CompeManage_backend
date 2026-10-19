# CompeManage 后端

基于 Go 的学科竞赛管理平台 API 服务，负责身份认证、权限校验、业务数据访问和外部师生数据同步。

## 技术与环境

- Go 1.24+
- Gin、GORM、JWT、Viper
- MySQL 8+
- Redis 6+
- Docker Compose（可选，用于启动本地 MySQL 和 Redis）

## 本地启动

### 1. 配置环境

复制示例环境变量文件：

```bash
cp .env.example .env
```

根据本地 MySQL、Redis 服务修改 `.env`。当前开发 Compose 的 MySQL root 默认密码是 `123456`，需将 `.env` 的 `DB_PASSWORD` 设为相同值，或同步修改 Compose。配置文件加载顺序为：环境变量覆盖 YAML 配置；启动时会尝试加载项目根目录的 `.env` 文件。

主要变量：

| 变量 | 用途 |
|---|---|
| `APP_ENV` / `ENV` | 选择配置环境；默认使用 dev |
| `DB_HOST`、`DB_PORT`、`DB_USER`、`DB_PASSWORD`、`DB_NAME` | MySQL 连接 |
| `REDIS_HOST`、`REDIS_PORT`、`REDIS_PASSWORD`、`REDIS_DB` | Redis 连接 |
| `SERVER_HOST`、`SERVER_PORT` | API 监听地址，默认端口 8080 |
| `JWT_SECRET` | JWT 签名密钥 |
| `CAS_CLIENT_ID`、`CAS_CLIENT_SECRET`、`CAS_REDIRECT_URI`、`CAS_FRONTEND_URL` | CAS 登录配置 |

开发配置文件为 `config/config-dev.yaml`。将 `APP_ENV=production` 映射为 `config/config-prod.yaml`；也可通过 `ENV` 显式指定环境名。当前开发 YAML 在仓库中受版本管理，公开发布前应核对其中的数据大厅凭据；若真实凭据曾提交，应先轮换。生产密钥、数据库密码或真实外部服务凭据不要提交到仓库。

### 2. 准备数据库和缓存

可以使用仓库提供的基础设施 Compose：

```bash
docker compose up -d
```

该文件只启动 MySQL 和 Redis，并创建 `CompeManage` 数据库。连接已有 MySQL 时，请先创建该数据库并确保应用账号有访问权限；启动时 GORM 会自动迁移模型并初始化角色、权限和种子数据。

### 3. 启动 API

```bash
go mod download
go run main.go
```

默认地址为 `http://localhost:8080`。健康检查：

```bash
curl http://localhost:8080/health
```

应用需从后端目录启动，以便按当前工作目录读取 `config/`。

## 生产 Docker 配置

生产配置包含数据库口令、JWT 密钥、数据大厅密钥和 CAS 凭据。仓库只跟踪不含真实密钥的示例文件，实际文件需在部署机器上自行创建：

```bash
cp docker-compose.prod.example.yml docker-compose.prod.yml
cp config/config-prod.example.yaml config/config-prod.yaml
```

PowerShell：

```powershell
Copy-Item docker-compose.prod.example.yml docker-compose.prod.yml
Copy-Item config/config-prod.example.yaml config/config-prod.yaml
```

编辑新建的两个文件，替换全部示例占位符和域名。Compose 的 `DB_PASSWORD` 要与 MySQL 的 `MYSQL_ROOT_PASSWORD` 相同；配置中的数据库地址和 Redis 地址应使用 Compose 服务名。实际文件 `docker-compose.prod.yml` 和 `config/config-prod.yaml` 已在 `.gitignore` 中，不能用示例值直接部署。

启动：

```bash
docker compose -f docker-compose.prod.yml up -d
```

生产 Compose 包含后端、MySQL 和 Redis。数据库数据保存在本地挂载目录，Redis 使用 AOF 持久化。部署前应配置强密码、限制数据库和 Redis 的公网访问，并安排数据库备份。

## 主要 API 模块

所有业务接口位于 `/api` 下；除登录、CAS 回调和健康检查等公开端点外，接口通过 JWT 和后端权限中间件保护。

| 前缀 | 功能 |
|---|---|
| `/api/login`、`/api/cas` | 用户密码登录与 CAS 统一身份认证 |
| `/api/comp` | 赛事目录、赛事年份和批量导入 |
| `/api/declare` | 赛事申报及审核 |
| `/api/reg` | 报名配置、报名审核、作品提交 |
| `/api/award` | 获奖填报、导入、导出、学生补录与审核 |
| `/api/review` | 评审任务、专家评分和结果 |
| `/api/summary` | 赛事总结 |
| `/api/statistics` | 统计看板 |
| `/api/notice` | 通知管理 |
| `/api/perm` | 权限、角色和校级管理员用户管理 |
| `/api/upload`、`/api/file` | 文件上传及受控下载 |

用户管理路由为 `GET /api/perm/member/list` 和 `PUT /api/perm/member/:id/role`。受保护接口需使用 `Authorization: Bearer <token>`。

## 角色与数据访问

系统授权角色包括校级管理员（`school_admin`）、院级管理员（`college_admin`）、赛事负责人（`competition_manager`）、教师（`teacher`）、学生（`student`）、专家（`expert`）和访客（`guest`）。自然身份字段与授权角色分开保存；权限判断和数据范围在后端执行。校级管理员可管理用户角色，院级管理员的数据范围关联管理学院；赛事负责人和专家根据负责赛事或分配的评审任务访问数据。

每个用户只能关联一个授权角色。历史数据升级前，先备份数据库并检查迁移预览：

```bash
go run ./cmd/user_role_migrate --mode dry-run
```

检查通过并做好备份后，根据迁移工具要求执行 `apply`，再执行 `verify`。工具支持通过显式确认标志执行迁移或回滚，具体参数见：

```bash
go run ./cmd/user_role_migrate --help
```

应用启动时会初始化测试数据。正式部署前检查 `database/seed.go` 中的测试用户创建逻辑，并确保默认口令、测试账号和外部集成凭据不会用于生产。

## 数据同步与后台任务

项目包含学校数据大厅的师生、学院和部门数据同步逻辑。外部数据同步依赖相应接口凭据和网络访问；未配置时相关同步无法正常工作。定时任务和间隔以 `main.go` 当前实现及日志为准。

## 开发命令

```bash
go test ./...
go test -race ./...
go vet ./...
go build -o compe-manage-api
```

压力测试脚本位于 `scripts/`，需先安装 k6：

```bash
k6 run scripts/stress-test.js
```

## 生产部署文件

- `docker-compose.prod.example.yml`：生产 Compose 示例
- `config/config-prod.example.yaml`：生产后端配置示例
- `.env.example`：本地开发环境变量示例

实际的生产配置文件应由部署人员本地创建并保管，不要提交到 Git。根目录的[项目 README](../README.md)和[前端 README](../CompeManage_frontend/README.md)提供整体开发说明。
