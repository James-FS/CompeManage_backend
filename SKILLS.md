# 后端开发规范

## 项目概述

基于 **Go + Gin + MySQL + GORM + Redis** 的 RESTful API 后端服务。

---

## 测试规范

### 工具

| 工具 | 用途 |
|------|------|
| `github.com/stretchr/testify` | assert/require 断言 |
| `github.com/DATA-DOG/go-sqlmock` | Mock GORM 数据库 |
| `net/http/httptest` | HTTP 接口测试 |
| `github.com/golang/mock` | 接口 Mock 生成 |

### 测试文件命名

```
xxx_controller.go → xxx_controller_test.go
utils/jwt.go      → utils/jwt_test.go
```

### 测试函数命名

```go
// 格式: Test函数名_场景
func TestMapAwardStatusToInt(t *testing.T) { ... }
func TestSubmitRegistration_成功(t *testing.T) { ... }
```

### 覆盖率目标

- 单元测试覆盖率 >= 85%
- 集成测试以 `TestIntegration*` 开头

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
            // test logic
        })
    }
}
```

### Mock Setup

```go
func TestService(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockRepo := mocks.NewMockRepository(ctrl)
    mockRepo.EXPECT().
        FindByID(gomock.Any(), "id").
        Return(&Entity{}, nil)

    svc := NewService(mockRepo)
    // test...
}
```

### Mock 生成

```bash
//go:generate mockgen -source=service.go -destination=service_mock.go -package=<pkg>
make generate
```

### 断言

```go
assert.NoError(t, err)
assert.Equal(t, expected, actual)
assert.Nil(t, result)
assert.NotNil(t, result)
assert.True(t, condition)
assert.Contains(t, slice, element)
```

---

## 实战经验

### Gin + sqlmock 组合写法

**setupDBMock**：每个测试函数独立创建 mock，避免子测试间共享状态导致 `all expectations were already fulfilled`。

```go
func setupDBMock(t *testing.T) sqlmock.Sqlmock {
    db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
    assert.NoError(t, err)
    gormDB, err := gorm.Open(mysql.New(mysql.Config{
        Conn:                      db,
        SkipInitializeWithVersion: true,
    }), &gorm.Config{})
    assert.NoError(t, err)
    database.DB = gormDB  // 注入全局 mock DB
    return mock
}
```

**httptest GET 辅助函数**：

```go
func buildNoticeGET(urlStr string) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
    req := httptest.NewRequest("GET", urlStr, nil)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Request = req
    return req, w, c
}
```

**httptest POST Form 辅助函数**：

```go
func buildNoticePOSTForm(body url.Values) (*http.Request, *httptest.ResponseRecorder, *gin.Context) {
    req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body.Encode()))
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Request = req
    return req, w, c
}
```

### c.Params 必须手动设置

`gin.CreateTestContext` 创建的 context **不包含 URL 路由参数**，所以调用 `c.Param("id")` 会拿到空值。

**需要在测试中手动注入路由参数**：

```go
c.Params = []gin.Param{{Key: "id", Value: "999"}}
```

### 子测试必须各自创建 mock

Table-driven 子测试共享外层 mock 会导致后续子测试失败（第一个子测试用完所有 mock expectation）。

**正确做法**：每个子测试内部调用 `setupDBMock(t)`：

```go
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        mock := setupDBMock(t)
        defer mock.ExpectationsWereMet()
        // ...
    })
}
```

### Count 查询的正则匹配

GORM 的 `Count()` 执行 `SELECT count(*) FROM ...`，和 `SELECT * FROM ...` 不同。sqlmock 默认精确匹配，需要用正则：

```go
mock.ExpectQuery("SELECT count\\(\\*\\) FROM `notices`").
    WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
```

### 每个测试结束后校验 mock expectation

```go
defer mock.ExpectationsWereMet()
```

---

## 已测试文件覆盖率

| 文件 | 函数 | 覆盖率 |
|------|------|--------|
| `file_controller.go` | `sanitizeFilename` | **100%** |
| `file_controller.go` | `UploadFile` | 88.9% |
| `file_controller.go` | `calcFileMD5` | 75.0% |
| `notice_controller.go` | `GetNoticeList` | 90.7% |
| `notice_controller.go` | `GetNoticeDetail` | 92.9% |
| `notice_controller.go` | `CreateNotice` | 94.7% |
| `notice_controller.go` | `CreateCompNotice` | 85.7% |
| `notice_controller.go` | `PublishNotice` | 95.0% |
| `notice_controller.go` | `UpdateNotice` | 89.2% |
| `notice_controller.go` | `DeleteNotice` | 93.8% |

---

## 可测试函数参考

### award_controller.go

| 函数 | 测试类型 |
|------|----------|
| `mapAwardStatusToInt` | 纯函数 |
| `mapAwardStatusFromInt` | 纯函数 |
| `getLeaderMember` | 纯函数 |
| `clearAwardCache` | Mock Redis |

### register_controller.go

| 函数 | 测试类型 |
|------|----------|
| `isValidTime` | 纯函数 |
| `checkUserIsAdmin` | Mock DB |

### middleware/auth.go

| 函数 | 测试类型 |
|------|----------|
| `RequirePermission` | Mock Redis + DB |

### datasource/datahall.go

| 函数 | 测试类型 |
|------|----------|
| `derefStr` | 纯函数 |
| `lastNChars` | 纯函数 |
| `hashPassword` | 纯函数 |

### statistics_controller.go

| 函数 | 测试类型 |
|------|----------|
| `calcPercent` | 纯函数 |
| `maxInt64` | 纯函数 |

---

## 运行测试

```bash
# 运行所有测试
go test -v -race ./...

# 运行指定文件测试
go test -v -race ./controllers/ -run "Notice"

# 查看覆盖率
go test -coverprofile=coverage.out ./controllers/
go tool cover -func=coverage.out

# 生成 HTML 覆盖率报告
go tool cover -html=coverage.out -o=coverage.html
```

---

## CI

CI 自动运行 `go test -v -race ./...`，参考 `.github/workflows/ci.yml`

---

## 其他

- 不建议单元测试的函数：`SyncStudents`、`updateExpiredComps`（依赖外部 API）、`file.Open()` 系统级错误分支
- 所有 `gin.Context` 处理器建议用 httptest 集成测试覆盖
- 参考 `controllers/file_controller_test.go` 和 `controllers/notice_controller_test.go` 的完整写法