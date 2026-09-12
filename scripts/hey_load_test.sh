#!/bin/bash
# hey_load_test.sh - 压力测试脚本
# 用法: ./hey_load_test.sh [URL] [请求数] [并发数] [TOKEN]
#
# 参考命令：
#
# --- 单机 Go + Redis 缓存（直接测 Go 端口）---
# ./hey_load_test.sh http://localhost:8080/api/notice/list 10000 500 "TOKEN"
#
# --- Nginx 负载均衡（3个Go实例）---
# ./hey_load_test.sh http://localhost:8088/api/notice/list 20000 1000 "TOKEN"
#
# --- 高压力测试（4k QPS，稳定成功率）---
# ./hey_load_test.sh http://localhost:8088/api/notice/list 20000 1000 "TOKEN"
#
# --- 报名接口测试 ---
# ./hey_load_test.sh http://localhost:8088/api/reg/list 10000 200 "TOKEN"

URL="${1:-http://localhost:8088/api/notice/list}"
N="${2:-10000}"
C="${3:-500}"
TOKEN="${4:-}"

if [ -z "$TOKEN" ]; then
    echo "请提供 Authorization Token 作为第四个参数"
    exit 1
fi

echo "========================================"
echo "HEY 压力测试"
echo "URL: $URL"
echo "请求数: $N"
echo "并发: $C"
echo "========================================"

hey -n "$N" -c "$C" -H "Authorization: Bearer $TOKEN" "$URL"
