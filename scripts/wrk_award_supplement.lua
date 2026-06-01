-- wrk_award_supplement.lua
-- 奖项补录接口压测脚本（POST + JSON）
-- 使用方式: wrk -t4 -c500 -d30s -s scripts/wrk_award_supplement.lua http://localhost:8080

-- 修改为你的实际 Token
local TOKEN = "YOUR_JWT_TOKEN_HERE"

wrk.method = "POST"
wrk.headers["Content-Type"] = "application/json"
wrk.headers["Authorization"] = "Bearer " .. TOKEN

-- 每次请求随机生成 teamName，避免被缓存
counter = 0
wrk.body = function()
    counter = counter + 1
    return string.format([[{
        "comp_id": 1,
        "team_name": "压测队伍_%d",
        "members": [
            {"name": "张三", "student_id": "2024001", "phone": "13800138000", "is_leader": 1, "year": 2024}
        ],
        "award_level": "一等奖",
        "award_name": "最佳创新奖",
        "award_date": "2026-05-01",
        "proof_url": "https://example.com/proof.pdf"
    }]], counter)
end
