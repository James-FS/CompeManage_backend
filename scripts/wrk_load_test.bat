@echo off
REM wrk_load_test.bat - Windows 压力测试脚本
REM 用法: wrk_load_test.bat [URL] [持续时间]
REM 示例: wrk_load_test.bat http://localhost:8080/api/award/list 30

set URL=%1
if "%URL%"=="" set URL=http://localhost:8080/api/award/list

set DURATION=%2
if "%DURATION%"=="" set DURATION=30s

set THREADS=4

echo ========================================
echo WRK 压力测试
echo URL: %URL%
echo Duration: %DURATION%
echo Threads: %THREADS%
echo ========================================

set CONCURRITIES=100 200 300 500 800 1000 1500 2000

for %%C in (%CONCURRITIES%) do (
  echo.
  echo ^^^>^^^> %%C 并发连接
  echo ----------------------------------------
  wrk -t%THREADS% -c%%C -d%DURATION% --latency %URL%
  echo.
)

echo ========================================
echo 压测完成
echo ========================================
