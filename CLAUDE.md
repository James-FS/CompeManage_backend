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

---

## 测试规范

### 工具

| 工具 | 用途 |
|------|------|
| `testify/assert` | 断言 |
| `go-sqlmock` | Mock GORM 数据库 |
| `net/http/httptest` | HTTP 接口测试 |
| `gomock` | 接口 Mock 生成 |

### 命名

- 测试文件：`xxx_controller_test.go`
- 测试函数：`Test函数名_场景`（如 `TestSubmitRegistration_成功`）
- 集成测试：`TestIntegration*`

### 覆盖率

- 单元测试覆盖率 >= 85%

### Table-Driven Tests

```go
func TestFunction(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {name: "valid input", input: "foo", expected: "bar"},
        {name: "empty input", input: "", wantErr: true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mock := setupDBMock(t)
            defer mock.ExpectationsWereMet()
            // test logic
        })
    }
}
```

### 实战经验

1. **setupDBMock**：每个子测试独立创建 mock，避免共享状态
2. **c.Params 手动设置**：`gin.CreateTestContext` 不包含路由参数
3. **Count 查询正则**：`mock.ExpectQuery("SELECT count\\(\\*\\) FROM ...")`
4. **defer mock.ExpectationsWereMet()**：每个测试结束后校验

### 辅助函数

```go
func setupDBMock(t *testing.T) sqlmock.Sqlmock {
    db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
    assert.NoError(t, err)
    gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
    assert.NoError(t, err)
    database.DB = gormDB
    return mock
}

func buildGET(urlStr string) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
    req := httptest.NewRequest("GET", urlStr, nil)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Request = req
    return req, w, c
}
```

### 运行测试

```bash
go test -v -race ./...
go test -coverprofile=coverage.out ./controllers/
go tool cover -func=coverage.out
```

### 已测试文件覆盖率

| 文件 | 函数 | 覆盖率 |
|------|------|--------|
| `file_controller.go` | `sanitizeFilename` | 100% |
| `file_controller.go` | `UploadFile` | 88.9% |
| `file_controller.go` | `calcFileMD5` | 75.0% |
| `notice_controller.go` | `GetNoticeList` | 90.7% |
| `notice_controller.go` | `GetNoticeDetail` | 92.9% |
| `notice_controller.go` | `CreateNotice` | 94.7% |
| `notice_controller.go` | `CreateCompNotice` | 85.7% |
| `notice_controller.go` | `PublishNotice` | 95.0% |
| `notice_controller.go` | `UpdateNotice` | 89.2% |
| `notice_controller.go` | `DeleteNotice` | 93.8% |
| `comp_controller.go` | `levelPrefix` | 100% |
| `comp_controller.go` | `resolveCompetitionYear` | 100% |
| `comp_controller.go` | `nextCompetitionCode` | 100% |
| `comp_controller.go` | `hasCompetitionStarted` | 100% |
| `comp_controller.go` | `resolveCollegeIDByName` | 100% |
| `comp_controller.go` | `GetCompetitionList` | 参数校验 |
| `comp_controller.go` | `CreateCompetition` | 参数校验 + 错误处理 |
| `comp_controller.go` | `DeleteCompetition` | 未找到 + 已开始 |
| `comp_controller.go` | `RestoreCompetition` | 未找到 + 未删除 |
| `comp_controller.go` | `GetCompetitionDetail` | 100% |
| `comp_controller.go` | `UpdateCompetition` | 参数校验 + 未找到 |
| `comp_controller.go` | `BatchDeleteCompetition` | 参数校验 + 已开始 |
| `comp_controller.go` | `GetCompetitionYears` | 100% |
| `comp_controller.go` | `GetManagerList` | 参数校验 + 角色查询 |
| `comp_controller.go` | `BatchImportCompetition` | 参数校验 + 学院不存在 |

### 可测试函数参考

| 文件 | 函数 | 测试类型 |
|------|------|----------|
| `award_controller.go` | `mapAwardStatusToInt` | 纯函数 |
| `award_controller.go` | `mapAwardStatusFromInt` | 纯函数 |
| `award_controller.go` | `getLeaderMember` | 纯函数 |
| `award_controller.go` | `clearAwardCache` | Mock Redis |
| `register_controller.go` | `isValidTime` | 纯函数 |
| `register_controller.go` | `checkUserIsAdmin` | Mock DB |
| `middleware/auth.go` | `RequirePermission` | Mock Redis + DB |
| `datasource/datahall.go` | `derefStr` | 纯函数 |
| `datasource/datahall.go` | `lastNChars` | 纯函数 |
| `datasource/datahall.go` | `hashPassword` | 纯函数 |
| `statistics_controller.go` | `calcPercent` | 纯函数 |
| `statistics_controller.go` | `maxInt64` | 纯函数 |

### 不建议单元测试的函数

- `SyncStudents`、`updateExpiredComps`（依赖外部 API）
- `file.Open()` 系统级错误分支

### CI

CI 自动运行 `go test -v -race ./...`，参考 `.github/workflows/ci.yml`
