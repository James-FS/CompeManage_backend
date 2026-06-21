# K6 压力测试配置

## 目标
- 目标 QPS: 5000
- 测试场景: 需要 JWT 认证的 API 接口

## 测试接口列表
1. `GET /api/notice/list` - 通知列表
2. `GET /api/comp/list` - 赛事列表
3. `GET /api/award/list` - 奖项列表
4. `GET /api/reg/my-reg` - 我的报名

## 运行方式

### 1. 本地快速测试 (单节点)
```bash
# 先运行服务
cd e:/CompeManage_backend/CompeManage_backend
go run main.go &

# 运行压测
k6 run scripts/stress-test.js
```

### 2. 高并发压测 (5K QPS)
```bash
k6 run scripts/stress-test.js --out influxdb=http://localhost:8086
```

### 3. 使用场景模式
```bash
k6 run scenarios-test.js
```

## 前置要求
1. 安装 k6: `brew install k6` / `choco install k6` / `winget install k6`
2. 可选: InfluxDB + Grafana 用于可视化

## 测试结果指标
- http_req_duration: 平均响应时间
- http_req_failed: 请求失败率
- iteration_duration: 迭代间隔