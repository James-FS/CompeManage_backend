# 学科竞赛管理系统后端

这是学科竞赛管理系统的 Go 后端服务，为前端提供认证、赛事目录、赛事申报、报名配置、报名审核、作品提交、专家评审、获奖填报、赛事总结、权限管理、文件上传下载和数据统计等 RESTful API。

线上演示：

```text
http://1.117.230.218/
```

前端仓库：

```text
https://github.com/James-FS/CompeManage_frontend
```

完整线上演示操作记录见前端仓库 `docs/demo-operation-log.md`。

## 技术栈

- Go 1.24
- Gin
- GORM
- MySQL 8
- Redis
- JWT
- Viper
- Lumberjack 日志滚动
- Excelize
- Docker Compose

## 主要能力

### 认证与权限

- 账号密码登录，成功后签发 JWT。
- CAS/OAuth2.0 统一身份认证入口。
- RBAC 权限模型，角色包括校级管理员、院级管理员、赛事负责人、老师、学生、专家、访客。
- 路由层使用 `AuthRequired` 和 `RequirePermission` 做接口级权限控制。

### 赛事管理

- 赛事目录查询、创建、编辑、删除、恢复、批量导入。
- 赛事负责人和年份维度查询。
- 学院侧赛事申报，校级侧审核。

### 报名与作品

- 报名规则配置。
- 学生报名、重新提交、报名状态查询。
- 报名审核、作品提交、作品审核。
- 支持学生、指导老师、队员等用户选择接口。

### 获奖、评审与总结

- 获奖填报、获奖模板导出、获奖名单导入。
- 学生获奖补录、获奖审核、批量审核。
- 专家评审任务分配、评审进度、评审结果确认。
- 赛事总结填写、查看和附件管理。

### 基础能力

- 健康检查：`GET /health`
- 通知公告管理。
- 学院、部门基础数据查询。
- 文件上传和受控下载。
- 数据统计看板。
- 赛事状态定时更新。

## 目录结构

```text
CompeManage_backend/
├── config/           # Viper 配置加载、环境变量绑定
├── controllers/      # HTTP 控制器
├── database/         # GORM 初始化、基础数据种子
├── datasource/       # DataHall 外部数据同步
├── logger/           # slog/lumberjack 日志配置
├── middleware/       # CORS、JWT 鉴权、权限校验、Redis 初始化
├── models/           # GORM 模型
├── routes/           # API 路由注册
├── scripts/          # 压测与辅助脚本
├── utils/            # JWT、响应封装、调度器、文件工具
├── Dockerfile
├── docker-compose.yml
├── docker-compose.prod.yml
└── main.go
```

## API 模块

| 模块 | 前缀 | 说明 |
| --- | --- | --- |
| 健康检查 | `/health` | 服务可用性检查 |
| 登录 | `/api/login` | 账号密码登录 |
| CAS | `/api/cas` | 统一身份认证登录与回调 |
| 文件 | `/api/upload`, `/api/file` | 文件上传、受控下载 |
| 通知 | `/api/notice` | 通知公告管理 |
| 权限 | `/api/perm` | 权限树、角色列表、角色授权 |
| 赛事 | `/api/comp` | 赛事目录管理 |
| 申报 | `/api/declare` | 学院申报与校级审核 |
| 报名 | `/api/reg` | 报名配置、报名审核、作品提交 |
| 获奖 | `/api/award` | 获奖填报、导入、审核 |
| 总结 | `/api/summary` | 赛事总结 |
| 统计 | `/api/statistics` | 数据看板 |
| 评审 | `/api/review` | 专家评审 |

## 演示账号

基础数据由 `database.InitData()` 初始化。密码均为 `123`。

| 角色 | 用户名 | 说明 |
| --- | --- | --- |
| 校级管理员 | `T2023001` | 拥有所有权限 |
| 院级管理员 | `T2023002` | 学院侧管理与审核 |
| 赛事负责人 | `T2023003` | 赛事管理、报名配置、报名审核 |
| 老师 | `T2023010` / `T2023011` / `T2023012` | 指导教师选择 |
| 学生 | `S2024001` | 报名、作品提交、获奖补录 |
| 专家 | `E2023001` / `E2023002` | 专家评审 |
| 访客 | `guest` | 无业务权限 |

## 配置说明

配置文件按环境加载：

```text
config/config-dev.yaml
config/config-prod.yaml
```

环境变量优先级高于 YAML。常用变量：

| 变量 | 说明 |
| --- | --- |
| `APP_ENV` | 运行环境，`production` 会加载 `config-prod.yaml` |
| `SERVER_PORT` | 后端端口，默认 `8080` |
| `DB_HOST` / `DB_PORT` | MySQL 地址 |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | MySQL 账号与数据库 |
| `REDIS_HOST` / `REDIS_PORT` | Redis 地址 |
| `JWT_SECRET` | JWT 签名密钥 |
| `CAS_CLIENT_ID` / `CAS_CLIENT_SECRET` | 统一认证配置 |
| `CAS_REDIRECT_URI` | CAS 回调地址 |
| `CAS_FRONTEND_URL` | 登录后跳转的前端地址 |
| `DATAHALL_ENABLED` | 是否启用 DataHall 外部同步，演示环境必须保持 `false` |
| `DATAHALL_KEY` / `DATAHALL_SECRET` | DataHall 密钥，禁止提交真实值 |

## DataHall 安全开关

`demo-deploy` 分支默认关闭外部数据同步：

```env
DATAHALL_ENABLED=false
DATAHALL_KEY=
DATAHALL_SECRET=
```

启动日志应包含：

```text
DataHall sync disabled
```

只有在真实内网生产环境、确认合规并配置好密钥后，才允许设置：

```env
DATAHALL_ENABLED=true
```

## Docker 部署

演示部署推荐使用 `demo-deploy` 分支：

```bash
git clone -b demo-deploy https://github.com/James-FS/CompeManage_backend.git
cd CompeManage_backend
sudo docker compose -f docker-compose.prod.yml up -d --build
```

查看容器：

```bash
sudo docker compose -f docker-compose.prod.yml ps
```

检查健康接口：

```bash
curl http://127.0.0.1:8080/health
```

查看日志：

```bash
sudo docker logs --tail=100 compemanage_app
```

停止服务：

```bash
sudo docker compose -f docker-compose.prod.yml down
```

## 本地开发

准备 MySQL 和 Redis 后，配置环境变量或修改 `config/config-dev.yaml`。

下载依赖：

```bash
go mod download
```

启动：

```bash
go run main.go
```

测试：

```bash
go test ./...
```

构建：

```bash
go build ./...
```

## Nginx 反向代理

前端部署在 Nginx 根路径，后端通过 `/api` 转发：

```nginx
location /api/ {
    proxy_pass http://127.0.0.1:8080/api/;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}

location /health {
    proxy_pass http://127.0.0.1:8080/health;
}
```

## 安全注意

- 不要提交真实 DataHall key/secret。
- 演示环境保持 `DATAHALL_ENABLED=false`。
- 生产环境必须替换 `JWT_SECRET`、MySQL 密码和 Redis 访问策略。
- MySQL、Redis 不建议暴露公网端口。
- 上传文件和下载接口应始终经过鉴权与权限校验。
