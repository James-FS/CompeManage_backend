# 学科竞赛管理系统 - 项目规范

## 项目概述

基于 **Go + Gin + MySQL + GORM** 的 RESTful API 后端服务。

**技术栈：**
- 框架：Gin (Web 框架)
- ORM：GORM
- 数据库：MySQL
- 认证：JWT
- 配置：Viper + godotenv

---

## 项目结构

```
CompeManage_backend/
├── config/          # 配置管理（环境变量读取、配置初始化）
├── controllers/     # 控制层（处理 HTTP 请求）
├── database/        # 数据库模块（连接、初始化、数据填充）
├── middleware/      # 中间件（CORS、认证、权限）
├── models/          # 数据模型（使用 GORM）
├── routes/          # 路由管理（API 端点定义）
├── utils/           # 工具函数（JWT、响应、日志）
├── static/          # 静态文件存储
├── main.go          # 应用入口
├── .env             # 环境变量配置
└── docker-compose.yml
```

---

## 命名规范

| 类型 | 规范 | 示例 |
|------|------|------|
| 目录 | 小写单数或复数 | `controllers`, `models` |
| 文件 | 下划线分隔 | `auth_controller.go`, `base_model.go` |
| 结构体 | PascalCase | `User`, `CompetitionDetail` |
| 变量/函数 | camelCase | `userID`, `getUserList` |
| 常量 | 全大写下划线分隔 | `SuccessCode`, `UnauthorizedCode` |
| 数据库表名字段 | 下划线分隔 | `user_roles`, `create_time` |

---

## API 设计规范

### 路由分组

```
r.GET("/health", ...)                                      # 公开接口
r.Group("/api/upload", middleware.AuthRequired())          # 上传（需登录）
r.Group("/api/notice", middleware.AuthRequired())         # 通知管理
r.Group("/api/comp", middleware.AuthRequired())           # 赛事管理
r.Group("/api/declare", middleware.AuthRequired())        # 申报管理
r.Group("/api/reg", middleware.AuthRequired())            # 报名管理
r.Group("/api/award", middleware.AuthRequired())          # 奖项管理
r.Group("/api/summary", middleware.AuthRequired())        # 总结管理
r.Group("/api/statistics", middleware.AuthRequired())     # 统计
r.Group("/api/perm", middleware.AuthRequired())           # 权限管理
```

### 权限控制

每个需要权限的路由使用 `middleware.RequirePermission(权限码)`：
```go
comp.GET("/list",
    controllers.GetCompetitionList,
    middleware.RequirePermission("comp:list"))
```

权限码格式：`资源:操作`，如 `notice:create`、`comp:delete`

### 响应格式

统一响应结构：
```json
{
    "code": 0,
    "message": "success",
    "data": {},
    "timestamp": 1234567890
}
```

业务错误码：
- `0` - 成功
- `400` - 参数错误
- `401` - 未授权
- `403` - 禁止访问
- `404` - 资源不存在
- `500` - 服务器错误
- `1001` - Token 过期
- `1002` - 手机号无效

---

## 代码规范

### 控制器规范

```go
func GetUserList(c *gin.Context) {
    // 1. 参数绑定与校验
    var input struct {
        Page   int    `form:"page"`
        Size   int    `form:"size"`
        Name   string `form:"name"`
    }
    if err := c.ShouldBindQuery(&input); err != nil {
        utils.BadRequest(c, "参数错误")
        return
    }

    // 2. 业务逻辑
    var users []models.User
    query := database.DB
    if input.Name != "" {
        query = query.Where("name LIKE ?", "%"+input.Name+"%")
    }

    // 3. 查询与响应
    query.Offset((input.Page - 1) * input.Size).Limit(input.Size).Find(&users)
    utils.Success(c, users)
}
```

### 模型规范

- 嵌入 `BaseModel` 获取公共字段（ID、CreatedAt、UpdatedAt、DeletedAt）
- 使用 GORM tag 定义列属性
- 敏感字段（如 Password）使用 `json:"-"` 忽略序列化
- 使用 `gorm.DeletedAt` 实现软删除

```go
type User struct {
    BaseModel
    Username string `gorm:"type:varchar(32);uniqueIndex;not null"`
    Password string `gorm:"type:varchar(255);not null" json:"-"`
    // ...
}
```

### 中间件规范

- 函数返回 `gin.HandlerFunc`
- 认证中间件从 `Authorization` 头解析 Bearer Token
- 权限中间件通过数据库查询验证权限

---

## 数据库规范

### 连接配置

```go
database.DB = dialector.Table("table_name").Session(&gorm.Session{})
```

### 公共字段

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| create_time | datetime | 创建时间（自动） |
| update_time | datetime | 更新时间（自动） |
| delete_time | datetime | 软删除标记 |

### 软删除

使用 GORM 的 `DeletedAt` 实现，查询自动过滤已删除记录。

---

## 配置规范

使用 Viper + godotenv，读取顺序：
1. `.env` 文件
2. 系统环境变量
3. 代码默认值

---

## 日志规范

使用 `log/slog` 包：
- 错误响应（code >= 400）记录 Warn 日志
- 成功响应记录 Info 日志

---

## Git 规范

- 分支：`main`（主分支）、`ywx_Develop`（开发分支）
- Commit Message：简洁描述变更内容
- 敏感文件不提交（.env、静态上传文件）
