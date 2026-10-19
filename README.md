# CompeManage 后端

基于 Go 的学科竞赛管理平台 API，负责认证、角色权限、数据范围校验、竞赛业务、文件访问和学校数据同步。

## 技术与环境

- Go 1.24+
- Gin、GORM、JWT、Viper、godotenv
- MySQL 8+
- Redis 6+
- Docker Compose（可选，用于本地数据库和缓存）

## 本地启动

### 配置

在后端仓库根目录复制环境变量模板：

```bash
cp .env.example .env
cp config/config-dev.example.yaml config/config-dev.yaml
```

Windows PowerShell 可使用 `Copy-Item` 复制以上两个文件。`config/config-dev.yaml` 是本地配置，已列入 `.gitignore`；请在本机填写数据大厅等需要的凭据，不要提交该文件。开发 Compose 的 MySQL root 默认密码为 `123456`；`.env` 中的 `DB_PASSWORD` 会覆盖 YAML 配置，需与 Compose 密码一致或同步修改 Compose。配置文件由 `APP_ENV` 或 `ENV` 选择，默认使用 `config/config-dev.yaml`。应用从当前工作目录读取 `config/`，因此命令需在后端目录运行。

| 变量 | 用途 |
|---|---|
| `APP_ENV`、`ENV` | 选择环境；`development` 对应 dev，`production` 对应 prod |
| `DB_HOST`、`DB_PORT`、`DB_USER`、`DB_PASSWORD`、`DB_NAME` | MySQL 连接 |
| `REDIS_HOST`、`REDIS_PORT`、`REDIS_PASSWORD`、`REDIS_DB` | Redis 连接 |
| `SERVER_HOST`、`SERVER_PORT` | 服务监听地址，默认端口 8080 |
| `JWT_SECRET` | JWT 签名密钥 |
| `CAS_CLIENT_ID`、`CAS_CLIENT_SECRET`、`CAS_REDIRECT_URI`、`CAS_FRONTEND_URL` | CAS 客户端设置 |

仓库只跟踪不含真实凭据的 `config/config-dev.example.yaml` 和 `config/config-prod.example.yaml`。本地实际配置（`config/config-dev.yaml`、`config/config-prod.yaml`）以及生产 Compose 文件不会提交到 Git。真实凭据若曾在历史提交中出现，应先轮换。

### 启动依赖服务及 API

仓库的 `docker-compose.yml` 只启动 MySQL 和 Redis，并创建 `CompeManage` 数据库：

```bash
docker compose up -d
go mod download
go run main.go
```

默认监听 `http://localhost:8080`。检查服务：

```bash
curl http://localhost:8080/health
```

第一次启动时，GORM 会迁移业务模型并初始化权限、角色、学院等基础数据。对于已经有数据的数据库，初始化会执行相应的幂等补齐逻辑。

## 生产 Docker 配置

仓库跟踪两个不含实际凭据的示例文件。首次部署时在后端目录复制为本地实际配置：

```bash
cp docker-compose.prod.example.yml docker-compose.prod.yml
cp config/config-prod.example.yaml config/config-prod.yaml
```

Windows PowerShell：

```powershell
Copy-Item docker-compose.prod.example.yml docker-compose.prod.yml
Copy-Item config/config-prod.example.yaml config/config-prod.yaml
```

将占位符和示例域名替换为部署值。Compose 中的 `DB_PASSWORD` 必须与 `MYSQL_ROOT_PASSWORD` 一致；数据库与 Redis 地址应使用 Compose 服务名。真实文件 `docker-compose.prod.yml` 和 `config/config-prod.yaml` 已列入 `.gitignore`，不要提交，也不要直接使用示例密码和地址。

```bash
docker compose -f docker-compose.prod.yml up -d
```

生产 Compose 启动后端、MySQL 和 Redis。数据库使用本地目录持久化，Redis 开启 AOF。正式部署还需由部署环境做好网络访问限制、凭据管理和数据库备份。

## API 模块

业务 API 以 `/api` 为前缀。用户名密码登录、CAS 登录回调和健康检查为公开入口；其余业务接口依路由配置使用 JWT、角色权限和资源范围校验。

| 路由前缀 | 主要能力 |
|---|---|
| `/api/login`、`/api/cas` | 用户名密码登录、CAS 统一身份认证 |
| `/api/comp` | 赛事目录查询、创建与编辑、批量导入、负责人查询、赛事年份 |
| `/api/declare` | 赛事申报创建、修改、提交、撤回、列表和审核 |
| `/api/reg` | 报名规则配置、报名提交和状态、报名审核、作品提交和审核、成员选择 |
| `/api/award` | 获奖导入与预览、团队/成员信息维护、导出模板、附件一/附件二/汇总表导出、学生补录和审核、指导获奖及变更记录 |
| `/api/review` | 可评审赛事、专家与任务管理、任务分配/初始化、进度和结果、专家评分 |
| `/api/summary` | 已结束赛事总结列表、详情和保存 |
| `/api/statistics` | 当前用户范围内的统计看板 |
| `/api/notice` | 通知列表、详情、创建、编辑、发布和删除 |
| `/api/perm` | 权限和角色查询、角色权限分配、校级管理员用户查询与角色设置 |
| `/api/upload`、`/api/file` | 文件上传、受控文件下载和预览 |

校级管理员用户管理接口是 `GET /api/perm/member/list` 和 `PUT /api/perm/member/:id/role`。其他请求携带 `Authorization: Bearer <token>`。

## 角色权限与数据范围

后端同时检查角色权限码和业务数据归属。前端传入的学院、赛事或用户 ID 不能覆盖服务端计算出的访问范围。

| 角色 | 主要操作 | 服务端数据范围 |
|---|---|---|
| `school_admin` | 全局赛事、用户和角色管理；全校申报/报名/获奖审核、专家评审管理、统计和导出 | 全校 |
| `college_admin` | 本院赛事申报与审核、报名/作品审核、获奖填报/审核/导出、评审任务管理、通知和统计 | 绑定的 `managed_college_id`。赛事、申报、报名、总结及统计按学院过滤；获奖、文件等接口另按提交学院或业务关系进行过滤。 |
| `competition_manager` | 负责赛事的报名配置/审核、作品审核、获奖填报/审核/导出、评审管理、通知和总结 | 主要按 `comp_directories.manager_id` 限制到本人负责的赛事；获奖导出还按负责人所属单位过滤填报记录。 |
| `student` | 浏览赛事、报名、团队及作品提交、查看本人获奖和补录申报 | 本人及本人参与的团队、报名、获奖和文件 |
| `teacher` | 浏览赛事和通知、作为指导教师参与业务、查看本人指导获奖 | 本人相关的指导获奖和允许访问的赛事、通知、文件 |
| `expert` | 查看分配任务、作品详情，提交或修改评分 | 分配给本人的评审任务、作品和记录 |
| `guest` | 默认无业务权限 | 无 |

院级管理员必须绑定一个管理学院。**院级管理员可以兼任赛事负责人**：赛事可将院管理员指定为负责人，但用户的授权角色仍是唯一的 `college_admin`，并不会增加第二个 `competition_manager` 角色。候选校验要求赛事属于该管理员的管理学院。对自己被指定负责的赛事，院管理员除院级查看/审核外，还能使用“本人负责赛事”权限编辑赛事、配置报名并填写总结；对同院其他负责人负责的赛事，不会获得这些负责人专属写权限。

人员自然身份（`identity_type`）与授权角色分开。每位用户只能关联一个授权角色，数据库有唯一约束；校级管理员可为用户设置角色，院级管理员角色需要同时选定管理学院。为避免误读：教师被选为赛事负责人时可能由系统调整为 `competition_manager`，院管理员被选为赛事负责人时会保留 `college_admin`。

权限树包含赛事、申报、报名、获奖、评审、总结、通知、基础数据和用户管理等操作权限。权限码授权不替代资源校验；服务端仍需验证目标赛事、学院、用户本人、团队成员或评审任务的归属。

## 获奖导入和导出

- 导入通过 Excel 模板、预览、数据校验和提交步骤完成；是否要求关联报名按系统设置处理。
- 可按学生/团队及其工号匹配记录；指导教师通过工号解析，支持在一个单元格中填写多名教师并使用常见分隔符。
- 校级管理员、院级管理员和赛事负责人可使用相应获奖导出接口；服务端分别按全校、学院提交归属或赛事负责人及其单位范围过滤。
- 附件导出包含获奖项目、获奖学生和上报汇总表；单位签章名称使用导出用户所属单位，必要时回退到管理学院名称。
- 指导教师获奖接口限制在本人指导项目；审核、补录、成员编辑和变更历史按各自角色和记录范围校验。

## 数据同步与定时任务

外部数据大厅用于同步学生、教职工、学院和部门数据，需要有效凭据和网络访问。当前主程序中的主要定时任务：

- 赛事状态更新：每 5 分钟。
- 权限缓存失效任务重试：每 1 分钟。
- 学生、教职工、学院和部门数据同步：每 6 小时。
- 研究生同步：当前暂停。

## 数据库迁移

应用启动会执行 GORM AutoMigrate。旧数据库升级到单角色数据模型前，请先备份并检查迁移报告：

```bash
go run ./cmd/user_role_migrate --mode dry-run
```

确认报告并做好备份后按工具提示执行。应用迁移：

```bash
go run ./cmd/user_role_migrate --mode apply --backup-table user_roles_backup_YYYYMMDD --confirm APPLY
go run ./cmd/user_role_migrate --mode verify
```

回滚需要显式确认：

```bash
go run ./cmd/user_role_migrate --mode rollback --backup-table user_roles_backup_YYYYMMDD --confirm ROLLBACK
```

工具还会校验用户角色数量、院管理员学院绑定和唯一索引。完成验证前保留备份表，不要跳过数据库备份。

## 种子数据与正式环境

首次初始化以及已有数据库的初始化补齐会创建保留的人工测试用户。正式部署前请检查 `database/seed.go` 的测试用户逻辑，确认测试账号、口令和权限不会用于生产。开发配置文件也应检查数据大厅凭据，已经公开的真实密钥应轮换。

## 常用开发命令

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

## 相关文档

- [项目总览与角色说明](../README.md)
- [前端说明](../CompeManage_frontend/README.md)
- [开发规范](CLAUDE.md)
