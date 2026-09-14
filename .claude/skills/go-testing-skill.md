---
name: testing
description: Testing patterns including table-driven tests, mock setup, and coverage requirements. Use when writing tests or setting up test infrastructure.
userInvokable: true
metadata:
  mcpmarket-version: 1.0.0
---
# Testing

**References:** [Examples](examples.md)

## Rules

| Rule | Value |
|------|-------|
| Unit tests | `TestFunctionName` |
| Integration tests | `TestIntegration*` |
| Coverage | 85% minimum |
| Tools | `testify/assert`, `gomock` |

## Mock Generation

> [Example](examples.md#mock-generation)

Add at top of file with interfaces:

```go
//go:generate mockgen -source=service.go -destination=service_mock.go -package=<pkg>
```

Run with:
```bash
make generate
```

## Table-Driven Tests

> [Example](examples.md#table-driven-tests)

Use for testing multiple scenarios:

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

## Mock Setup

> [Example](examples.md#mock-setup)

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

## Assertions

> [Example](examples.md#assertions)

Use `testify/assert`:

```go
assert.NoError(t, err)
assert.Equal(t, expected, actual)
assert.Nil(t, result)
assert.NotNil(t, result)
assert.True(t, condition)
assert.Contains(t, slice, element)
```
