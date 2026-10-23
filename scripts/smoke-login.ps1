# 三角色登录冒烟：/health + 学生/教师/管理员登录 + 各取一个受保护接口
# 用法：pwsh scripts/smoke-login.ps1 [-BaseURL http://127.0.0.1:8080]
param(
    [string]$BaseURL = "http://127.0.0.1:8080"
)

$ErrorActionPreference = "Stop"
$failed = 0

function Check([string]$name, [scriptblock]$block) {
    try {
        & $block | Out-Null
        Write-Host "PASS  $name"
    } catch {
        Write-Host "FAIL  $name -> $($_.Exception.Message)"
        $script:failed++
    }
}

Check "health" {
    $r = Invoke-WebRequest "$BaseURL/health" -UseBasicParsing -TimeoutSec 8
    $j = $r.Content | ConvertFrom-Json
    if ($j.code -ne 200) { throw "health code=$($j.code)" }
}

$roles = @(
    @{ name = "管理员"; user = "900001"; pass = "Pw@yy03"; expectRole = "school_admin"; probe = "/api/statistics/dashboard" },
    @{ name = "教师";   user = "900007"; pass = "Pw@yy03"; expectRole = "teacher";      probe = "/api/comp/list?page=1&page_size=5" },
    @{ name = "学生";   user = "900006"; pass = "Pw@yy03"; expectRole = "student";      probe = "/api/reg/my-reg?page=1&size=5" }
)

foreach ($role in $roles) {
    $token = $null
    Check "$($role.name)登录($($role.user))" {
        $body = @{ username = $role.user; password = $role.pass } | ConvertTo-Json
        $r = Invoke-WebRequest "$BaseURL/api/login" -Method POST -Body $body -ContentType 'application/json' -UseBasicParsing -TimeoutSec 8
        $j = $r.Content | ConvertFrom-Json
        if ($j.code -ne 200) { throw "login code=$($j.code) msg=$($j.message)" }
        $script:token = $j.data.token
        if (-not $script:token) { throw "响应缺少 token" }
        if ($j.data.userInfo.role -ne $role.expectRole) { throw "角色不符: $($j.data.userInfo.role) != $($role.expectRole)" }
    }
    if ($token) {
        Check "$($role.name)受保护接口 $($role.probe)" {
            $r = Invoke-WebRequest "$BaseURL$($role.probe)" -Headers @{ Authorization = "Bearer $token" } -UseBasicParsing -TimeoutSec 8
            $j = $r.Content | ConvertFrom-Json
            if ($j.code -ne 200) { throw "probe code=$($j.code)" }
        }
    }
}

if ($failed -gt 0) {
    Write-Host "`n冒烟失败: $failed 项"
    exit 1
}
Write-Host "`n冒烟全部通过"
