# 学科竞赛管理系统 - 后端服务

## 📋 项目概述

学科竞赛管理系统后端是一个基于 **Go + Gin + MySQL + GORM** 构建的 RESTful API 服务，为学科竞赛管理平台提供核心业务支持。

**技术栈：**
- **框架**：Gin (高性能 Web 框架)
- **ORM**：GORM (Go 语言 ORM 库)
- **数据库**：MySQL
- **认证**：JWT (JSON Web Token)
- **配置管理**：Viper

---

## 🏗️ 项目架构

### 目录结构

```
CompeManage_backend/
├── config/           # 配置管理模块（环境变量读取、配置初始化）
├── controllers/      # 控制层（处理 HTTP 请求）
├── database/         # 数据库模块（数据库连接、初始化）
├── middleware/       # 中间件（CORS、认证等）
├── models/          # 数据模型（定义数据结构）
├── routes/          # 路由管理（API 端点定义）
├── utils/           # 工具函数（JWT、响应处理等）
├── main.go          # 应用入口
├── .env             # 环境变量配置（本地开发）
├── .env.example     # 环境变量示例
└── go.mod           # Go 模块定义
```

---

## 🔧 环境配置

### 前置条件

- **Go 1.25.0** 或更高版本
- **MySQL 5.7** 或更高版本
- **git** (版本控制)

### 安装依赖

```bash
# 1. 克隆项目
git clone <repository-url>
cd CompeManage_backend

# 2. 下载 Go 依赖
go mod download

# 或使用国内镜像加速（可选）
go env -w GOPROXY=https://goproxy.cn,direct
go mod download
```

### 环境变量配置

#### 方式一：复制 .env.example（推荐）

```bash
cp .env.example .env
```

然后编辑 `.env` 文件，配置相应的环境变量。

#### 方式二：手动创建 .env 文件

在项目根目录创建 `.env` 文件，内容如下：

```env
# 数据库配置
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=your_password
DB_NAME=CompeManage

# 服务器配置
SERVER_PORT=8080
SERVER_HOST=0.0.0.0

# JWT 密钥（用于用户认证）
JWT_SECRET=your-jwt-secret-key-here

# 其他配置
APP_ENV=development
```

### 配置说明

| 变量名 | 说明 | 默认值 | 备注 |
|--------|------|--------|------|
| `DB_HOST` | MySQL 数据库主机 | localhost | - |
| `DB_PORT` | MySQL 数据库端口 | 3306 | - |
| `DB_USER` | MySQL 用户名 | root | - |
| `DB_PASSWORD` | MySQL 密码 | 空 | ⚠️ 必须配置 |
| `DB_NAME` | 数据库名称 | CompeManage | - |
| `SERVER_PORT` | 服务器监听端口 | 8080 | - |
| `SERVER_HOST` | 服务器监听地址 | 0.0.0.0 | - |
| `JWT_SECRET` | JWT 签名密钥 | your-jwt-secret | 生产环境必须修改 |

#### ⚠️ 配置重点

配置系统采用**环境变量优先**的方式：

1. **读取顺序**：`.env` 文件 → 系统环境变量 → 代码默认值
2. **优先级**：`.env` 文件中的值 **优先级最高**
3. **若要使用 .env 配置**：务必确保 `.env` 文件在项目根目录，且 `godotenv.Load()` 能成功加载

**配置初始化流程**：

```go
// 1. 加载 .env 文件
godotenv.Load()

// 2. 初始化配置（从环境变量读取）
config.Init()

// 3. 使用配置
dbPassword := config.GetString("database.password")
```

---

## 🚀 快速开始

### 1. 准备数据库

```bash
# 创建数据库
mysql -u root -p -e "CREATE DATABASE CompeManage;"

# 导入表结构（如有 SQL 文件）
mysql -u root -p CompeManage < database.sql
```

### 2. 启动服务

```bash
# 开发环境运行
go run main.go

# 或编译后运行
go build -o compe-manage-api
./compe-manage-api
```

### 3. 验证服务

```bash
# 测试健康检查接口
curl http://localhost:8080/health

# 预期响应
{
    "code": 0,
    "data": {
        "status": "ok"
    },
    "message": "success"
}
```

---

## 📦 核心模块说明

### config 配置管理

**职责**：集中管理所有配置参数

```go
// 读取配置
port := config.GetString("server.port")
dbHost := config.GetString("database.host")
```

**关键函数**：
- `Init()` - 初始化配置，从环境变量读取
- `GetString(key)` - 获取字符串配置
- `GetInt(key)` - 获取整数配置

### database 数据库模块

**职责**：初始化数据库连接、提供全局 DB 实例

```go
// 获取数据库实例
db := database.GetDB()

// 执行查询（使用 GORM）
db.Find(&users)
```

**特点**：
- 使用 GORM ORM 简化数据操作
- 配置了连接池（最大 100 个连接）
- 开发环境打印 SQL 日志便于调试

### middleware 中间件

**当前实现**：
- **CORS** 中间件：允许跨域请求

```go
// 注册中间件
r.Use(middleware.CORS())
```

**配置说明**：
- 开发环境允许所有来源 (`*`)
- 生产环境应配置具体的允许域名

### routes 路由管理

**职责**：定义所有 API 端点

```go
// 注册路由
routes.SetupRoutes(r)
```

**当前端点**：
- `GET /health` - 健康检查

### utils 工具函数

**包含模块**：
- `jwt.go` - JWT 令牌生成和验证
- `response.go` - 统一响应格式

```go
// 统一响应格式
utils.Success(c, data)    // 成功响应
utils.Error(c, code, msg) // 错误响应
```

---

## 📄 许可证

MIT License

---

## 👥 贡献者

欢迎提交 Issue 和 Pull Request！
