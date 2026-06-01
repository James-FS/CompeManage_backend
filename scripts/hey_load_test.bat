@echo off
REM hey_load_test.bat - 压力测试脚本
REM 用法: hey_load_test.bat [URL] [请求数] [并发数] [TOKEN]
REM
REM 参考命令：
REM
REM --- 单机 Go + Redis 缓存（直接测 Go 端口）---
REM hey_load_test.bat http://localhost:8080/api/notice/list 10000 500 "TOKEN"
REM
REM --- Nginx 负载均衡（3个Go实例）---
REM hey_load_test.bat http://localhost:8088/api/notice/list 20000 1000 "TOKEN"
REM
REM --- 高压力测试（4k QPS，稳定成功率）---
REM hey_load_test.bat http://localhost:8088/api/notice/list 20000 1000 "TOKEN"
REM
REM --- 报名接口测试 ---
REM hey_load_test.bat http://localhost:8088/api/reg/list 10000 200 "TOKEN"

set URL=%~1
if "%URL%"=="" set URL=http://localhost:8088/api/notice/list

set N=%~2
if "%N%"=="" set N=10000

set C=%~3
if "%C%"=="" set C=500

set TOKEN=%~4
if "%TOKEN%"=="" (
    echo 请提供 Authorization Token 作为第四个参数
    exit /b 1
)

echo ========================================
echo HEY 压力测试
echo URL: %URL%
echo 请求数: %N%
echo 并发: %C%
echo ========================================

hey -n %N% -c %C% -H "Authorization: Bearer %TOKEN%" "%URL%"
