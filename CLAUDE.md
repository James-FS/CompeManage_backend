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
| `net/http/httptest` | HTTP 接口测试 + Mock 外部服务 |
| `gomock` | 接口 Mock 生成 |

### 命名

- 测试文件：`xxx_controller_test.go`
- 测试函数：`Test函数名_场景`（如 `TestFindOrCreateUser_NewUser`）
- 集成测试：`TestIntegration*`

### 覆盖率

- 单元测试覆盖率 >= 85%

### 写完单元测试后的必做步骤

每写完一个 `xxx_controller_test.go` 文件后，必须执行以下步骤：

1. **运行测试**：`go test -v ./controllers/ -run "^TestXxx"` 确认全部通过
2. **总结经验**：将遇到的问题和解决方案写入本文件的「实战经验」章节
3. **更新覆盖率表**：在「已测试文件覆盖率」中新增对应函数的测试结果
4. **更新可测试函数参考**：将已测试的函数从「可测试函数参考」中移除

经验总结应包含：
- GORM 查询的实际行为（如 Preload 顺序、事务包装）
- Mock 期望的正确写法
- 踩坑点和解决方案

### 测试策略

| 函数类型 | 测试方式 | 示例 |
|----------|----------|------|
| 纯函数 | 直接调用，table-driven | `levelPrefix`, `getAttr` |
| 数据库函数 | sqlmock | `findOrCreateUser`, `nextCompetitionCode` |
| HTTP 处理器 | httptest + sqlmock | `DeleteCompetition`, `GetCompetitionDetail` |
| 外部 API 调用 | httptest.NewServer mock | `exchangeCodeForToken`, `fetchCasUserProfile` |
| 依赖 Redis | 只测试错误路径，成功路径需集成测试 | `GetCompetitionList`, `clearCompListCache` |

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

### 辅助函数模板

```go
// 数据库 Mock
func setupDBMock(t *testing.T) sqlmock.Sqlmock {
    db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
    assert.NoError(t, err)
    gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
    assert.NoError(t, err)
    database.DB = gormDB
    return mock
}

// GET 请求构造
func buildGET(urlStr string) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
    req := httptest.NewRequest("GET", urlStr, nil)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Request = req
    return req, w, c
}

// POST JSON 请求构造
func buildPOSTJSON(body interface{}) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
    jsonBytes, _ := json.Marshal(body)
    req := httptest.NewRequest("POST", "/", bytes.NewBuffer(jsonBytes))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Request = req
    return req, w, c
}

// Mock 外部 HTTP 服务
func mockHTTPServer(handler http.HandlerFunc) *httptest.Server {
    return httptest.NewServer(handler)
}
```

### 实战经验

#### 1. GORM 事务处理

GORM 的 `Create` 使用事务，需要 mock Begin/Commit/Rollback：

```go
mock.ExpectQuery("SELECT .* FROM `users`").WillReturnRows(...)
mock.ExpectBegin()
mock.ExpectExec("INSERT INTO `users`").WillReturnResult(sqlmock.NewResult(1, 1))
mock.ExpectCommit()
```

创建失败时：
```go
mock.ExpectBegin()
mock.ExpectExec("INSERT INTO `users`").WillReturnError(fmt.Errorf("duplicate key"))
mock.ExpectRollback()
```

GORM 的 `Update`、`Save`、`Delete`（含 `Unscoped().Delete()`）也使用事务：
```go
mock.ExpectBegin()
mock.ExpectExec("UPDATE `xxx`").WillReturnResult(sqlmock.NewResult(1, 1))
mock.ExpectCommit()

mock.ExpectBegin()
mock.ExpectExec("DELETE FROM `xxx`").WillReturnResult(sqlmock.NewResult(1, 1))
mock.ExpectCommit()
```

#### 2. GORM Preload 关联查询

Preload 会先查关联表，再查主表：

```go
// 主查询
mock.ExpectQuery("SELECT .* FROM `users`").WillReturnRows(...)
// Preload 关联表（顺序：先中间表，再目标表）
mock.ExpectQuery("SELECT .* FROM `user_roles`").WillReturnRows(...)
mock.ExpectQuery("SELECT .* FROM `roles`").WillReturnRows(...)
```

**嵌套 Preload 顺序**：GORM 按声明顺序逐层展开，同层内按关联类型排序（BelongsTo 优先）。
例如 `Preload("Register").Preload("Register.Leader").Preload("Register.Competition").Preload("Register.Members")` 的实际查询顺序：
1. `registers`（主关联）
2. `users`（BelongsTo，即 Leader）
3. `reg_members`（HasMany，即 Members）
4. `comp_directories`（BelongsTo，即 Competition）

若 mock 期望顺序不对，会报 `all expectations were already fulfilled` 错误。

#### 3. 路由参数手动注入

`gin.CreateTestContext` 不包含路由参数，必须手动设置：

```go
_, w, c := buildGET("/api/comp/delete/1")
c.Params = []gin.Param{{Key: "id", Value: "1"}}
DeleteCompetition(c)
```

#### 4. Count 查询正则

GORM 的 `Count()` 生成 `SELECT count(*)`，需要转义：

```go
mock.ExpectQuery("SELECT count\\(\\*\\) FROM `comp_directories`").
    WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
```

#### 5. Mock 外部 HTTP 服务

使用 `httptest.NewServer` 模拟外部 API：

```go
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    assert.Equal(t, "POST", r.Method)
    assert.Equal(t, "/api/token", r.URL.Path)
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"token": "test_token"})
}))
defer server.Close()

cfg := config.CasConfig{ServerURL: server.URL}
```

#### 6. 配置隔离

测试全局配置时，保存/恢复避免污染：

```go
func TestWithConfig(t *testing.T) {
    origConfig := config.AppConfig
    defer func() { config.AppConfig = origConfig }()
    
    config.AppConfig.Cas = config.CasConfig{ServerURL: "http://test.com"}
    // test logic...
}
```

#### 7. 依赖 Redis 的函数

`clearCompListCache` 等函数依赖 Redis，单元测试只覆盖错误路径：

```go
// ✅ 可以测试：参数错误、记录不存在等（不触发 Redis）
func TestDeleteCompetition_NotFound(t *testing.T) { ... }
func TestDeleteCompetition_Started(t *testing.T) { ... }

// ❌ 跳过：成功后会调用 clearCompListCache
// func TestDeleteCompetition_Success(t *testing.T) { ... }
```

#### 8. Mock 期望顺序

每个子测试独立创建 mock，避免共享状态：

```go
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        mock := setupDBMock(t)  // 独立 mock
        defer mock.ExpectationsWereMet()
        // ...
    })
}
```

#### 9. 错误断言

```go
assert.Error(t, err)
assert.Contains(t, err.Error(), "expected message")
assert.NoError(t, err)
assert.Equal(t, http.StatusBadRequest, w.Code)
```

#### 10. InternalServerError 响应内容

`utils.InternalServerError(c, "获取学院数据失败", err)` 在响应体中不会暴露原始消息，实际返回：
```json
{"code":500,"message":"服务器内部错误，请稍后重试","timestamp":...}
```
断言时应使用 `assert.Contains(t, w.Body.String(), "服务器内部错误")` 而非函数内部的错误消息。类似地，`BadRequest`、`Unauthorized` 等会暴露消息，但 `InternalServerError` 不会。

#### 11. GORM Association().Replace() 行为

`db.Model(&role).Association("Permissions").Replace(&perms)` 在一个事务中执行：
1. `UPDATE roles SET update_time=?` （更新父模型时间戳）
2. `INSERT INTO permissions ... ON DUPLICATE KEY UPDATE` （Upsert 关联对象）
3. `DELETE FROM role_permissions` （删除旧关联）
4. `INSERT INTO role_permissions` （插入新关联）

Mock 时需要在一个事务中按顺序 mock 这些操作。

#### 12. 嵌套结构体 binding:"required" 校验

当请求体结构体包含嵌套结构体且嵌套字段有 `binding:"required"` 时，`ShouldBindJSON` 会在绑定阶段就校验嵌套字段。例如 `ApplicationReq` 包含 `MemberReq Leader`，而 `MemberReq` 的 `Name` 和 `StuID` 有 `binding:"required"`，测试时必须提供合法的 `Leader` 数据，否则会在参数绑定阶段就返回 400。

```go
// ❌ 错误：缺少 Leader 数据，ShouldBindJSON 会失败
body := ApplicationReq{CompID: 1, TeamName: "test"}

// ✅ 正确：提供完整的 Leader 数据
body := ApplicationReq{
    CompID:   1,
    TeamName: "test",
    Leader:   MemberReq{Name: "张三", StuID: "2022001"},
}
```

#### 13. GORM Preload 空结果与 AutoSave

当 GORM `Preload` 查询返回空结果时，关联字段为零值结构体（如 `CompDirectory{ID: 0}`）。如果后续调用 `Updates()` 或 `Save()`，GORM 的 AutoSave 机制可能会尝试 INSERT 关联对象。测试中如果 mock 了 Preload 查询但返回空行，需要注意后续操作可能产生的额外 SQL。

### 运行测试

```bash
# 运行所有测试
go test -v -race ./...

# 运行指定文件的测试（避免匹配到其他文件）
go test -v ./controllers/ -run "^Test(LevelPrefix|ResolveCompetitionYear)"

# 查看覆盖率
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
| `cas_controller.go` | `casProfileResponse.getAttr` | 100% |
| `cas_controller.go` | `redirectWithError` | 100% |
| `cas_controller.go` | `CasLogin` | 100% |
| `cas_controller.go` | `exchangeCodeForToken` | 100% |
| `cas_controller.go` | `fetchCasUserProfile` | 100% |
| `cas_controller.go` | `findOrCreateUser` | 100% |
| `cas_controller.go` | `CasCallback` | 部分场景 |
| `auth_controller.go` | `checkPassword` | 100% |
| `auth_controller.go` | `Login` | 100% |
| `award_controller.go` | `mapAwardStatusToInt` | 100% |
| `award_controller.go` | `mapAwardStatusFromInt` | 100% |
| `award_controller.go` | `getLeaderMember` | 100% |
| `award_controller.go` | `DateOnly.UnmarshalJSON` | 100% |
| `award_controller.go` | `SearchCompetition` | 参数校验 + 成功 + 空结果 |
| `award_controller.go` | `GetAwardAuditDetail` | 未找到 |
| `award_controller.go` | `PassAwardAudit` | 未找到 + 已审核 |
| `award_controller.go` | `RejectAwardAudit` | 未找到 + 已审核 + 缺原因 |
| `award_controller.go` | `BatchPassAwardAudit` | 空ID + 无可审核记录 |
| `award_controller.go` | `BatchRejectAwardAudit` | 空ID + 缺原因 + 无可审核记录 |
| `award_controller.go` | `GetStudentMyAwardList` | 未登录 + 无效状态 + 无效赛事ID + 成功 |
| `award_controller.go` | `GetAwardCompList` | 成功 |
| `college_controller.go` | `GetCollegeList` | 成功 + 空列表 + DB错误 |
| `declare_controller.go` | `CreateDeclare` | 参数错误 + 未授权 + 成功 + DB错误 |
| `declare_controller.go` | `GetDeclareDetail` | 未找到 + DB错误 + 成功 |
| `declare_controller.go` | `UpdateDeclare` | 参数错误 + 未找到 + 禁止(已提交/已通过) + 成功(草稿/已驳回) |
| `declare_controller.go` | `SubmitDeclare` | 未找到 + 禁止(已提交/已通过) + 成功(草稿/已驳回) |
| `declare_controller.go` | `RevokeDeclare` | 未找到 + 禁止(草稿/已通过) + 成功 |
| `declare_controller.go` | `GetMyDeclares` | 参数错误 + 未授权 + 成功 |
| `declare_controller.go` | `GetMyPendingDeclares` | 参数错误 + 未授权 |
| `declare_controller.go` | `GetMyPublishedDeclares` | 参数错误 + 未授权 |
| `declare_controller.go` | `DeleteDeclare` | 未找到 + 禁止(非草稿) + 成功 |
| `declare_controller.go` | `GetPendingDeclares` | 参数错误 + 成功 |
| `declare_controller.go` | `AuditDeclare` | 参数错误 + 无效状态 + 未授权 + 未找到 + 禁止(非已提交) + 驳回成功 |
| `declare_controller.go` | `GetAllDeclares` | 参数错误 + 成功 |
| `declare_controller.go` | `GetAuditedDeclares` | 参数错误 + 成功 |
| `permission_controller.go` | `GetAllPermissions` | 成功 + 空列表 + DB错误 |
| `permission_controller.go` | `GetAllRoles` | 成功 + 空列表 |
| `permission_controller.go` | `AssignPermissions` | 参数错误 + 空body + 角色不存在 + 成功(有权限/空权限) + DB错误 |
| `register_controller.go` | `isValidTime` | 100% (5 cases) |
| `register_controller.go` | `checkUserIsAdmin` | 100% (true + false + DB error) |
| `register_controller.go` | `GetRegConfig` | 参数缺失 + 格式错误 + 未找到 + 无配置 + 成功 |
| `register_controller.go` | `GetRegDetail` | 参数缺失 + 格式错误 + 未找到 |
| `register_controller.go` | `AuditRegister` | 参数错误 + 非法状态 + 缺原因 + 未找到 + 免审核 + 已审核 + 无权 |
| `register_controller.go` | `GetMyRegStatus` | 参数缺失 + 格式错误 + 未登录 + DB错误 + 未报名 |
| `register_controller.go` | `SubmitWork` | 参数错误 + 未登录 + 未找到 + 非成员 |
| `register_controller.go` | `ResubmitRegistration` | 参数错误 + 未登录 + 未找到 + 已通过 + 配置不存在 + 附件/老师/赛道校验 + 时间校验 |
| `register_controller.go` | `SubmitRegistration` | 参数错误 + 未登录 + 未找到 + 未配置 + 时间校验 + 赛道/附件/老师校验 + 负责人不存在 + 重复报名 + DB错误 |
| `register_controller.go` | `GetRegList` | 未登录 |
| `register_controller.go` | `GetMyRegList` | 未登录 + DB错误 |
| `register_controller.go` | `GetUserList` | 参数错误 |
| `statistics_controller.go` | `calcPercent` | 100% (8 cases) |
| `statistics_controller.go` | `maxInt64` | 100% (6 cases) |
| `statistics_controller.go` | `GetStatisticsDashboard` | 未登录 |
| `review_controller.go` | `GetReviewCompList` | Count错误 + 空结果 + 有数据 + 默认分页 |
| `review_controller.go` | `GetExpertList` | Count错误 + 空结果 + 关键词+学院过滤 + 默认分页 |
| `review_controller.go` | `GetReviewTaskList` | 缺comp_id + 格式错误 + 空结果 + 有数据 |
| `review_controller.go` | `AssignReviewTask` | 参数错误 + 未登录 + 赛事不存在 + 成功 |
| `review_controller.go` | `InitReviewTasks` | 参数错误 + 赛事不存在 + 无任务 + 无作品 |
| `review_controller.go` | `DeleteReviewTask` | ID无效 + 未找到 + 有记录无force + 强制删除 + status=0删除 |
| `review_controller.go` | `GetReviewProgress` | 缺comp_id + 格式错误 + 赛事不存在 + 空结果 |
| `review_controller.go` | `GetReviewResultList` | 缺comp_id + 格式错误 + 赛事不存在 + 空结果 + 异常检测 |
| `review_controller.go` | `ConfirmReviewResult` | 参数错误 + 配置不存在 + 评审未结束 + 未完成 + 已有获奖 |
| `review_controller.go` | `GetMyReviewTasks` | 未登录 + 空结果 + 有数据 + 默认分页 |
| `review_controller.go` | `GetMyReviewWorks` | 未登录 + 缺task_id + 格式错误 + 未找到 + 空结果 + 状态过滤 + 有数据 |
| `review_controller.go` | `GetReviewWorkDetail` | 未登录 + regId无效 + 缺task_id + task_id无效 + 任务不存在 + 记录不存在 + 成功 |
| `review_controller.go` | `SubmitReview` | 参数错误 + 未登录 + 分数越界 + 记录不存在 + 已评审 + 未开始 + 已结束 |
| `review_controller.go` | `UpdateReview` | ID无效 + 参数错误 + 未登录 + 分数越界 + 记录不存在 + 未评审 + 未开始 + 已结束 |
| `review_controller.go` | `calcReviewStatus` | 无任务 + 全0 + 全1 + 全3 + 混合 |
| `review_controller.go` | `syncReviewTasks` | 新增 + 移除未初始化 + 跳过已初始化 + 强制关闭 |

### 可测试函数参考

| 文件 | 函数 | 测试类型 |
|------|------|----------|
| `award_controller.go` | `clearAwardCache` | Mock Redis |
| `middleware/auth.go` | `RequirePermission` | Mock Redis + DB |
| `datasource/datahall.go` | `derefStr` | 纯函数 |
| `datasource/datahall.go` | `lastNChars` | 纯函数 |
| `datasource/datahall.go` | `hashPassword` | 纯函数 |

### 不建议单元测试的函数

- `SyncStudents`、`updateExpiredComps`（依赖外部 API）
- `file.Open()` 系统级错误分支

### CI

CI 自动运行 `go test -v -race ./...`，参考 `.github/workflows/ci.yml`

#### 14. GORM 软删除 vs 硬删除

当模型嵌入 `BaseModel`（包含 `gorm.DeletedAt`）时，GORM 的 `Delete()` 执行软删除而非硬删除：

```go
// 期望（错误）：
mock.ExpectExec("DELETE FROM `review_tasks`")

// 实际 SQL（正确）：
mock.ExpectExec("UPDATE `review_tasks` SET `delete_time`")
```

`Unscoped().Delete()` 才会执行真正的 `DELETE FROM`。测试中所有涉及 `Delete` 的操作都需要使用 `UPDATE ... SET delete_time` 模式。

同样，`tx.Where(...).Delete(&models.Xxx{})` 也是软删除：
```go
// 期望：
mock.ExpectExec("UPDATE `review_records` SET `delete_time`")
```
