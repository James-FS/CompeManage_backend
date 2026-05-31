import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// 自定义指标
const errorRate = new Rate('errors');
const responseTime = new Trend('response_time');

const BASE_URL = 'http://localhost:8080';

// ============================================
// 方式1: 使用单点 Token (简单测试)
// ============================================
const SINGLE_TOKEN = 'YOUR_TOKEN_HERE';

// ============================================
// 方式2: 批量用户登录获取 Token (推荐)
// ============================================
// 测试账号列表 (格式: username:password)
const TEST_USERS = [
    'admin:123456',
    'teacher:123456',
    'student:123456',
];

// Token 缓存
let tokenCache = [];

// 登录获取 Token
function login(username, password) {
    const res = http.post(`${BASE_URL}/api/login`, JSON.stringify({
        username: username,
        password: password,
    }), {
        headers: { 'Content-Type': 'application/json' },
    });

    if (res.status === 200) {
        const data = res.json();
        return data.data?.token;
    }
    return null;
}

// 初始化: 预登录获取 Token
export function setup() {
    console.log('正在预登录获取测试 Token...');

    for (const user of TEST_USERS) {
        const [username, password] = user.split(':');
        const token = login(username, password);
        if (token) {
            tokenCache.push(token);
            console.log(`登录成功: ${username}`);
        } else {
            console.log(`登录失败: ${username}`);
        }
    }

    console.log(`共获取 ${tokenCache.length} 个有效 Token`);
    return { tokens: tokenCache };
}

// 获取随机的有效 Token
function getRandomToken(data) {
    if (data.tokens && data.tokens.length > 0) {
        const token = data.tokens[Math.floor(Math.random() * data.tokens.length)];
        return token;
    }
    return SINGLE_TOKEN;
}

// 测试通知列表
function testNoticeList(token) {
    const res = http.get(`${BASE_URL}/api/notice/list?page=1&size=20`, {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`,
        },
    });

    check(res, {
        'notice status 200': (r) => r.status === 200,
    });
    errorRate.add(res.status !== 200);
    responseTime.add(res.timings.duration);
}

// 测试赛事列表
function testCompList(token) {
    const res = http.get(`${BASE_URL}/api/comp/list?page=1&size=20`, {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`,
        },
    });

    check(res, {
        'comp status 200': (r) => r.status === 200,
    });
    errorRate.add(res.status !== 200);
    responseTime.add(res.timings.duration);
}

// 测试奖项列表
function testAwardList(token) {
    const res = http.get(`${BASE_URL}/api/award/list?page=1&size=20`, {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`,
        },
    });

    check(res, {
        'award status 200': (r) => r.status === 200,
    });
    errorRate.add(res.status !== 200);
    responseTime.add(res.timings.duration);
}

// 测试我的报名
function testMyReg(token) {
    const res = http.get(`${BASE_URL}/api/reg/my-reg?page=1&size=20`, {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`,
        },
    });

    check(res, {
        'my-reg status 200': (r) => r.status === 200,
    });
    errorRate.add(res.status !== 200);
    responseTime.add(res.timings.duration);
}

// 压测配置: 5K QPS
export let options = {
    scenarios: {
        // 目标: 5000 QPS
        // 4 个接口混合, 每个接口约 1250 QPS
        // 假设平均响应时间 50ms, 则需要: 1250 * 0.05 = 62.5 并发
        // 考虑余量, 设置 100 VUs 持续 60 秒

        sustained_load: {
            executor: 'constant-vus',
            vus: 100,
            duration: '60s',
        },

        // 阶梯压测
        ramp_up_load: {
            executor: 'ramping-vus',
            startVUs: 50,
            stages: [
                { duration: '20s', target: 100 },
                { duration: '20s', target: 200 },
                { duration: '20s', target: 300 },
            ],
        },
    },

    thresholds: {
        http_req_duration: ['p(95)<500'],
        http_req_failed: ['rate<0.01'],
        errors: ['rate<0.05'],
    },
};

// 主函数
export default function(data) {
    const token = getRandomToken(data);

    // 随机选择接口
    const rand = Math.floor(Math.random() * 4);
    switch (rand) {
        case 0: testNoticeList(token); break;
        case 1: testCompList(token); break;
        case 2: testAwardList(token); break;
        case 3: testMyReg(token); break;
    }

    sleep(0.01);
}