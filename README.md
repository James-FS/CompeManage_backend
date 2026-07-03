# 学科竞赛管理系统 - 后端服务

基于 Go 的学科竞赛管理平台后端，提供赛事管理、报名、作品提交、专家评审、获奖管理等核心业务 API。

## 技术栈

| 类别 | 技术 | 版本 |
|------|------|------|
| 语言 | Go | 1.24 |
| Web 框架 | Gin | v1.11.0 |
| ORM | GORM | v1.31.1 |
| 数据库 | MySQL | 8.0 |
| 缓存 | Redis (go-redis) | v9.18.0 |
| 认证 | JWT (golang-jwt) + CAS SSO | v5.3.0 |
| 配置管理 | Viper + godotenv | v1.21.0 |
| 日志 | log/slog + lumberjack | — |
| Excel 处理 | excelize | v2.10.0 |
| 密码加密 | bcrypt (golang.org/x/crypto) | v0.47.0 |
| 性能分析 | pprof (gin-contrib) | v1.5.3 |

## 环境要求

- Go 1.24+
- MySQL 8.0+
- Redis 6.0+
- Docker & Docker Compose（可选）

## 快速开始

### 1. 安装依赖

```bash
cd CompeManage_backend
go mod download

# 国内镜像加速（可选）
go env -w GOPROXY=https://goproxy.cn,direct
go mod download
```

### 2. 配置环境变量

复制 `.env.example` 为 `.env`，按需修改：

```env
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=your_password
DB_NAME=CompeManage
SERVER_PORT=8080
SERVER_HOST=0.0.0.0
JWT_SECRET=your-jwt-secret-key
APP_ENV=development
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
CAS_CLIENT_ID=
CAS_CLIENT_SECRET=
CAS_REDIRECT_URI=http://localhost:8080/api/cas/callback
CAS_FRONTEND_URL=http://localhost:5219
```

**配置加载优先级**：环境变量 > `.env` 文件 > YAML 配置文件 (`config/config-{env}.yaml`) > 代码默认值

通过 `APP_ENV` 或 `ENV` 选择配置文件：`development` → `config-dev.yaml`，`production` → `config-prod.yaml`。

### 3. 准备数据库

```bash
mysql -u root -p -e "CREATE DATABASE CompeManage CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
```

数据库表在首次启动时通过 GORM AutoMigrate 自动创建，种子数据（角色、权限、初始用户）自动填充。

### 4. 启动服务

```bash
go run main.go
# 服务启动在 http://localhost:8080
```

### 5. 验证

```bash
curl http://localhost:8080/health
# {"code":0,"message":"success","data":{"status":"ok"}}
```

## Docker 部署

### 开发环境（仅基础设施）

```bash
docker-compose up -d    # 启动 MySQL + Redis
go run main.go          # 本地运行后端
```

### 生产环境

```bash
docker-compose -f docker-compose.prod.yml up -d
```

| 服务 | 镜像 | 端口 | 说明 |
|------|------|------|------|
| app | `ywxx252324/compemanage:latest` | 8080 | Go 后端 |
| mysql-db | `mysql:8.0` | 3306 | 数据库，数据持久化 |
| redis | `redis:latest` | 6379 | 缓存，AOF 持久化 |

### 手动构建

```bash
docker build -t compemanage-backend .
docker run -p 8080:8080 compemanage-backend
```

多阶段构建：`golang:1.24-alpine` 编译静态二进制 → `alpine:latest` 运行，仅包含二进制和配置目录。

## 可用命令

| 命令 | 说明 |
|------|------|
| `go run main.go` | 开发模式运行 |
| `go build -o compe-manage-api` | 编译二进制 |
| `go test -v -race ./...` | 运行所有单元测试 |
| `go test -v ./controllers/ -run "^TestXxx"` | 运行指定测试 |
| `go test -coverprofile=coverage.out ./controllers/` | 生成覆盖率报告 |
| `go tool cover -func=coverage.out` | 查看覆盖率详情 |
| `k6 run scripts/stress-test.js` | 运行压力测试 |

## 项目结构

```
CompeManage_backend/
├── config/                  # 配置管理
│   ├── config.go            # 配置结构体与初始化 (Viper)
│   ├── config-dev.yaml      # 开发环境配置
│   └── config-prod.yaml     # 生产环境配置
├── controllers/             # 控制层
│   ├── auth_controller.go   # 用户名密码登录
│   ├── cas_controller.go    # CAS SSO 认证
│   ├── comp_controller.go   # 赛事管理
│   ├── declare_controller.go # 赛事申报
│   ├── register_controller.go # 报名管理
│   ├── award_controller.go  # 获奖管理
│   ├── notice_controller.go # 通知管理
│   ├── summary_controller.go # 赛事总结
│   ├── statistics_controller.go # 数据统计
│   ├── review_controller.go # 专家评审
│   ├── file_controller.go   # 文件上传下载
│   ├── permission_controller.go # 权限管理
│   └── college_controller.go # 基础数据
├── database/                # 数据库模块
│   ├── db.go                # GORM 初始化、连接池、AutoMigrate
│   └── seed.go              # 种子数据 (幂等)
├── datasource/              # 外部数据同步
│   ├── datahall.go          # 数据大厅 API 核心逻辑
│   ├── datahall_college.go  # 学院同步
│   ├── datahall_department.go # 部门同步
│   ├── datahall_staff.go    # 教职工同步
│   └── datahall_postgrad.go # 研究生同步
├── logger/                  # 结构化日志
│   └── logger.go            # slog + 异步写入 + 轮转
├── middleware/              # 中间件
│   ├── auth.go              # JWT 认证 + RBAC 权限校验
│   ├── cors.go              # 跨域配置
│   └── redis.go             # Redis 客户端
├── models/                  # 数据模型
│   ├── base.go              # 公共字段
│   ├── User.go              # 用户
│   ├── Role.go              # 角色
│   ├── Permission.go        # 权限
│   ├── College.go           # 学院
│   ├── Department.go        # 部门
│   ├── CompDirectory.go     # 赛事目录
│   ├── CompDetail.go        # 赛事详情
│   ├── CompDeclaration.go   # 赛事申报
│   ├── Register.go          # 报名记录
│   ├── RegMember.go         # 报名成员
│   ├── Award.go             # 获奖记录
│   ├── Summary.go           # 赛事总结
│   ├── notice.go            # 通知
│   ├── file_record.go       # 文件记录
│   ├── ReviewTask.go        # 评审任务
│   └── ReviewRecord.go      # 评审记录
├── routes/                  # 路由注册
│   └── routes.go
├── utils/                   # 工具函数
│   ├── jwt.go               # JWT 生成与解析
│   ├── response.go          # 统一响应格式
│   └── scheduler.go         # 定时任务
├── static/                  # 上传文件存储
├── scripts/                 # 压力测试与部署脚本
├── main.go                  # 应用入口
├── Dockerfile               # 多阶段构建
├── docker-compose.yml       # 开发环境
└── docker-compose.prod.yml  # 生产环境
```

## 启动流程

```
加载 .env → 读取 YAML 配置 → 初始化日志
→ 连接数据库 (GORM, AutoMigrate 16 个模型)
→ 填充种子数据 (幂等)
→ 连接 Redis → 注册 pprof → 注册 CORS 中间件
→ 注册路由 (75 个端点)
→ 启动后台定时任务 → 启动 HTTP 服务 (端口 8080)
```

**后台定时任务**：
- 赛事状态调度器：每 5 分钟，自动流转赛事状态
- 数据同步调度器：每 6 小时，从学校数据大厅同步学生、教职工、学院、部门数据

## 核心业务模块

### 赛事管理 (`/api/comp`)

赛事全生命周期管理，支持批量导入、软删除与恢复。

**赛事编号**：`{级别前缀}{年份}{3位序号}`（校级=X, 省级=S, 国家级=G, 国际级=I）

**状态流转**：

```
草稿 (0) ──定时任务──→ 已发布 (1) ──定时任务──→ 已结束 (2)
```

- 草稿→已发布：报名开始时间到达时自动流转
- 已发布→已结束：报名和提交截止时间均过时自动流转

### 赛事申报 (`/api/declare`)

院级管理员申报赛事，校级管理员审核。审核通过后自动创建赛事目录。

```
草稿 → 提交 → 审核 (通过/驳回)
                  ├─ 通过 → 自动创建赛事目录
                  └─ 驳回 → 可重新提交
```

### 报名管理 (`/api/reg`)

报名配置、学生报名、报名审核、作品提交。支持个人赛和团队赛，可配置是否需要附件、指导老师、报名审核。

### 获奖管理 (`/api/award`)

管理员批量导入获奖名单（Excel 模板），学生自主申报获奖，管理员审核。支持批量通过/驳回。

### 专家评审 (`/api/review`)

管理员初始化评审任务 → 分配专家 → 专家打分 → 管理员查看进度和结果 → 确认获奖。

### 通知管理 (`/api/notice`)

竞赛通知发布，支持富文本内容和附件，关联赛事详情。

### 赛事总结 (`/api/summary`)

赛后总结填写，支持费用明细（JSON）和附件上传。

### 数据统计 (`/api/statistics`)

数据看板，汇总赛事、报名、获奖等统计数据。

## API 设计

### 认证方式

所有 `/api/*` 接口（除公开接口外）需要携带 JWT Token：

```
Authorization: Bearer <token>
```

JWT 有效期 24 小时，载荷包含 `user_id`、`username`、`role_code`。

### 响应格式

```json
{
    "code": 0,
    "message": "success",
    "data": {},
    "timestamp": 1715491200
}
```

| 错误码 | 含义 |
|--------|------|
| 0 | 成功 |
| 400 | 参数错误 |
| 401 | 未授权 |
| 403 | 禁止访问 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |
| 1001 | Token 过期 |
| 1002 | 手机号无效 |

错误响应自动记录 Warn 日志，成功响应记录 Info 日志。

### 公开接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/health` | 健康检查 |
| POST | `/api/login` | 用户名密码登录 |
| GET | `/api/cas/login` | CAS SSO 登录跳转 |
| GET | `/api/cas/callback` | CAS SSO 回调 |
| GET | `/api/college/list` | 学院列表 |
| GET | `/api/department/list` | 部门列表 |

### 认证接口

#### 赛事管理 `/api/comp`

| 方法 | 路径 | 权限码 | 说明 |
|------|------|--------|------|
| GET | `/list` | `comp:list` | 赛事列表 |
| POST | `/create` | `comp:create` | 创建赛事 |
| POST | `/batch-import` | `comp:batch-import` | 批量导入 |
| DELETE | `/:id` | `comp:delete` | 删除（软删除） |
| POST | `/batch-delete` | `comp:batch-delete` | 批量删除 |
| PUT | `/:id/restore` | `comp:restore` | 恢复 |
| GET | `/manager/list` | `manager:list` | 负责人列表 |
| GET | `/years` | `comp:years:list` | 年份列表 |
| GET | `/:id` | `comp:detail` | 详情 |
| PUT | `/:id` | `comp:update` | 更新 |

#### 赛事申报 `/api/declare`

| 方法 | 路径 | 权限码 | 说明 |
|------|------|--------|------|
| POST | `` | `declare:create` | 创建 |
| GET | `/:id` | `declare:get` | 详情 |
| PUT | `/:id` | `declare:update` | 更新 |
| POST | `/:id/submit` | `declare:submit` | 提交 |
| POST | `/:id/revoke` | `declare:revoke` | 撤回 |
| GET | `/my/list` | `declare:list` | 我的申报 |
| GET | `/my/pending` | `declare:list` | 待处理 |
| GET | `/my/published` | `declare:list` | 已发布 |
| DELETE | `/:id` | `declare:delete` | 删除 |
| GET | `/pending/list` | `declare:pending-list` | 待审核（校级） |
| GET | `/audited/list` | `declare:audited-list` | 已审核（校级） |
| POST | `/audit` | `declare:audit` | 审核 |
| GET | `/all` | `declare:all-declares` | 全部申报 |

#### 报名管理 `/api/reg`

| 方法 | 路径 | 权限码 | 说明 |
|------|------|--------|------|
| POST | `/config` | `reg:config:edit` | 保存配置 |
| GET | `/config/get` | `reg:config:view` | 获取配置 |
| POST | `/submit` | `reg:config:submit` | 提交报名 |
| PUT | `/resubmit` | `reg:resubmit` | 重新提交 |
| PUT | `/work-submit` | `reg:my-reg:submit` | 提交作品 |
| GET | `/list` | `reg:audit:list` | 报名列表 |
| GET | `/detail` | `reg:audit:detail` | 报名详情 |
| PUT | `/audit` | `reg:audit:update` | 审核报名 |
| GET | `/status` | `reg:status` | 报名状态 |
| GET | `/my-reg` | `reg:my-reg` | 我的报名 |
| GET | `/user/list` | `reg:user:list` | 用户列表 |
| GET | `/work/audit/comp/list` | `reg:audit:list` | 作品审核赛事 |
| GET | `/work/audit/student/list` | `reg:audit:detail` | 作品审核学生 |

#### 获奖管理 `/api/award`

| 方法 | 路径 | 权限码 | 说明 |
|------|------|--------|------|
| GET | `/list` | `award:list` | 赛事列表 |
| GET | `/comp-awards` | `award:comp:list` | 赛事获奖 |
| POST | `/import` | `award:import` | 导入获奖 |
| GET | `/export-template` | — | 下载模板 |
| GET | `/comp/list` | — | 搜索赛事 |
| GET | `/student/my-awards` | `award:student:my-list` | 我的获奖 |
| POST | `/student/supplement` | — | 学生申报 |
| GET | `/audit/list` | — | 审核列表 |
| GET | `/audit/detail/:id` | — | 审核详情 |
| PUT | `/audit/:id/pass` | `award:audit` | 通过 |
| PUT | `/audit/:id/reject` | `award:audit` | 驳回 |
| PUT | `/audit/batch/pass` | `award:audit` | 批量通过 |
| PUT | `/audit/batch/reject` | `award:audit` | 批量驳回 |

#### 专家评审 `/api/review`

**管理员端：**

| 方法 | 路径 | 权限码 | 说明 |
|------|------|--------|------|
| GET | `/comp/list` | `review:comp:list` | 可评审赛事 |
| GET | `/expert/list` | `review:expert:list` | 专家列表 |
| GET | `/task/list` | `review:task:list` | 任务列表 |
| POST | `/task/assign` | `review:task:assign` | 分配任务 |
| POST | `/task/init` | `review:task:init` | 初始化任务 |
| DELETE | `/task/:id` | `review:task:delete` | 删除任务 |
| GET | `/progress` | `review:progress` | 评审进度 |
| GET | `/result/list` | `review:result:list` | 评审结果 |
| POST | `/result/confirm` | `review:result:confirm` | 确认结果 |

**专家端：**

| 方法 | 路径 | 权限码 | 说明 |
|------|------|--------|------|
| GET | `/my/tasks` | `review:my:list` | 我的任务 |
| GET | `/my/works` | `review:my:works` | 待评审作品 |
| GET | `/my/works/:regId` | `review:my:work:detail` | 作品详情 |
| POST | `/submit` | `review:my:submit` | 提交评审 |
| PUT | `/submit/:id` | `review:my:update` | 修改评审 |

#### 通知管理 `/api/notice`

| 方法 | 路径 | 权限码 | 说明 |
|------|------|--------|------|
| GET | `/list` | `notice:list` | 列表 |
| GET | `/:id` | `notice:detail` | 详情 |
| POST | `/comp/create` | `notice:create` | 创建 |
| PUT | `/:id/publish` | `notice:publish` | 发布 |
| PUT | `/:id` | `notice:update` | 更新 |
| DELETE | `/:id` | `notice:delete` | 删除 |

#### 其他

| 方法 | 路径 | 权限码 | 说明 |
|------|------|--------|------|
| GET | `/api/summary/list` | — | 总结列表 |
| GET | `/api/summary/:id` | — | 总结详情 |
| POST | `/api/summary/:id` | — | 保存总结 |
| GET | `/api/statistics/dashboard` | — | 数据看板 |
| GET | `/api/perm/permission/list` | `perm:list` | 权限列表 |
| GET | `/api/perm/role/list` | `role:list` | 角色列表 |
| POST | `/api/perm/role/assign_perm` | `perm:assign` | 分配权限 |
| POST | `/api/upload` | — | 文件上传 |
| GET | `/api/file/download/:type/:filename` | — | 文件下载 |

## 角色权限体系

### RBAC 模型

系统采用三级权限树（目录 → 子目录 → 叶子权限），通过 Redis 缓存加速校验。

```
竞赛管理
├── 竞赛目录 (comp:list, comp:create, comp:delete, comp:update, ...)
├── 赛事申报 (declare:create, declare:submit, declare:audit, ...)
├── 获奖管理 (award:list, award:import, award:student:my-list, ...)
└── 赛事总结 (summary:list, summary:detail, summary:edit)

报名管理
├── 报名配置 (reg:config:edit, reg:config:view)
├── 报名审核 (reg:audit:list, reg:audit:detail, reg:audit:update)
└── 报名提交 (reg:config:submit, reg:status, reg:my-reg, ...)

通知管理 (notice:list, notice:create, notice:publish, ...)

系统管理
├── 权限管理 (perm:list, role:list, perm:assign)
└── 基础数据 (college:list, upload:file)

专家评审 (review:comp:list, review:task:assign, review:my:submit, ...)
```

### 角色定义

| 角色 | 标识 | 说明 |
|------|------|------|
| 校级管理员 | `school_admin` | 全部权限 |
| 院级管理员 | `college_admin` | 本院赛事申报、审核、通知管理、评审管理 |
| 赛事负责人 | `competition_manager` | 赛事查看、报名配置/审核、获奖导入、通知管理、评审管理 |
| 教师 | `teacher` | 只读访问 |
| 学生 | `student` | 报名提交、获奖查看/申报 |
| 专家 | `expert` | 评审任务查看与打分 |
| 访客 | `guest` | 无权限 |

### 权限校验流程

```
请求 → AuthRequired (JWT 解析) → RequirePermission (权限码)
                                        │
                                        ├── Redis 命中 → 返回
                                        └── Redis 未命中 → MySQL 查询 → 写入 Redis (TTL 5min)
```

## 数据模型

### 公共字段

```go
type BaseModel struct {
    ID        uint           `gorm:"primaryKey"`
    CreatedAt time.Time      `gorm:"column:create_time;autoCreateTime"`
    UpdatedAt time.Time      `gorm:"column:update_time;autoUpdateTime"`
    DeletedAt gorm.DeletedAt `gorm:"column:delete_time;index"` // 软删除
}
```

### 模型关系

```
User ──many2many── Role ──many2many── Permission

CompDirectory ──1:1── CompDetail
CompDirectory ──N:1── User (Manager)
CompDirectory ──N:1── College
CompDirectory ──1:1── CompDeclaration

Register ──N:1── CompDirectory / User (Leader) / User (Advisor)
Register ──1:N── RegMember

Award ──N:1── Register / User (Auditor)
Summary ──1:1── CompDirectory

ReviewTask ──N:1── User (Expert)
ReviewRecord ──N:1── ReviewTask / Register
```

### 关键模型字段

**CompDirectory**：赛事编号 (unique)、名称、类别、级别、状态 (0/1/2)、来源 (校级创建/来自申报)、负责人、所属学院

**CompDetail**：报名/提交/赛事时间窗口、参赛类型 (个人/团队)、团队人数限制、年级要求、附件要求、指导老师要求、是否需要报名审核、赛道配置 (JSON)、奖项等级 (JSON)、是否需要评审

**Register**：赛事 ID、队长 ID、团队名称、赛道、状态 (待审核/通过/驳回/补充待审/补充通过/补充驳回)、报名附件、作品附件

**Award**：赛事 ID、报名 ID (unique)、奖项等级、获奖名称、状态 (草稿/待审核/通过/驳回)、来源 (导入/学生申报)

**ReviewTask**：赛事 ID、专家 ID (unique 复合索引)、状态 (已分配/待评审/评审中/已完成)

**ReviewRecord**：任务 ID、报名 ID (unique 复合索引)、评分、评语、状态 (未评审/已评审)

## 缓存策略

| 缓存项 | Key 格式 | TTL | 说明 |
|--------|----------|-----|------|
| 赛事列表 | `cache:comp_list:{request_md5}` | 5 分钟 | 增删改后自动清除 |
| 权限校验 | `perm:{userID}:{permCode}` | 5 分钟 | 正负结果均缓存，防止穿透 |
| 数据大厅 Token | `datahall:access_token` | 7000 秒 | 外部 API Token |

## 外部数据同步

系统通过学校数据大厅 API 自动同步师生数据，启动后 30 秒首次执行，之后每 6 小时同步一次。

**同步逻辑**：
1. 获取 Token（Redis 缓存优先）
2. 分页拉取数据（每页 500 条，按年级过滤）
3. 增量更新：新用户自动创建并分配角色，已有用户仅在字段变化时更新

**同步模块**：教职工、研究生、学院、部门数据（本科生通过独立同步逻辑）

## 认证机制

### 开发环境

用户名 + 密码登录（`POST /api/login`），bcrypt 验证密码，签发 JWT（HS256，24h 有效）。

### 生产环境

CAS 统一身份认证（OAuth2.0 授权码模式）：

```
前端 → /api/cas/login → CAS 登录页 → 回调 /api/cas/callback
     → 用授权码换取 Token → 获取用户信息 → 查找/创建本地用户
     → 签发 JWT → 重定向到前端
```

## 日志

`slog` 结构化 JSON 日志，异步写入（缓冲区 10000），lumberjack 自动轮转：

| 环境 | 文件 | 级别 | 最大大小 | 保留份数 |
|------|------|------|----------|----------|
| 开发 | `./logger/app.logger` | debug | 100MB | 5 |
| 生产 | `logs/prod.log` | info | 100MB | 30 |

## CI/CD

GitHub Actions 流水线，触发条件：push/PR 到 `main` 或 `develop`。

```
test (go test -race)
  ↓
build (Docker 构建 + Trivy 安全扫描)
  ↓
docker (推送到 Docker Hub，仅 main/ywx_develop)
  ↓
deploy (SSH 部署到生产服务器，仅 main)
```

## 压力测试

项目包含 k6 压力测试脚本，目标 QPS 5000。

```bash
# 本地快速测试
k6 run scripts/stress-test.js

# 高并发 + InfluxDB 监控
k6 run scripts/stress-test.js --out influxdb=http://localhost:8086

# hey 简单负载测试
bash scripts/hey_load_test.sh
```

监控指标：`http_req_duration`（响应时间）、`http_req_failed`（失败率）、`iteration_duration`。

项目还提供 Nginx 负载均衡配置和多实例 Docker Compose 用于高并发场景。

## 单元测试

使用 `testify` 断言 + `go-sqlmock` Mock 数据库，目标覆盖率 >= 85%。

```bash
go test -v -race ./...                                    # 全部测试
go test -v ./controllers/ -run "^TestLogin"               # 指定测试
go test -coverprofile=coverage.out ./controllers/         # 覆盖率报告
go tool cover -html=coverage.out                          # 浏览器查看
```

### 测试策略

| 函数类型 | 测试方式 |
|----------|----------|
| 纯函数 | table-driven tests |
| 数据库函数 | go-sqlmock |
| HTTP 处理器 | httptest + go-sqlmock |
| 外部 API | httptest.NewServer Mock |
| Redis 依赖 | 仅测试错误路径 |

### 注意事项

- GORM 的 `Create`/`Update`/`Delete` 均使用事务，Mock 需 `ExpectBegin` + `ExpectCommit`
- GORM 软删除执行 `UPDATE SET delete_time` 而非 `DELETE FROM`
- `InternalServerError` 在响应中隐藏原始错误，断言使用通用错误文本
- 每个子测试独立创建 Mock，避免状态污染
