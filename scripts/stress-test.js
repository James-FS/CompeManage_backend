import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// 自定义指标
const errorRate = new Rate('errors');
const responseTime = new Trend('response_time');

// 测试配置 - 针对 5K QPS 优化
const BASE_URL = 'http://localhost:8080';

// 测试用的 Token (通过 /api/login 获取的有效 Token)
const TEST_TOKEN = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxLCJ1c2VybmFtZSI6IlQyMDIzMDAxIiwicm9sZV9jb2RlIjoic2Nob29sX2FkbWluIiwiaXNzIjoiY29tcGVfYmFja2VuZCIsImV4cCI6MTc3ODk4NDczOCwiaWF0IjoxNzc4ODk4MzM4fQ.FzUIiIATvkTOmt_KeQYH7NcfWVXeheD9llk-2Rwpryg';

// 请求头
const headers = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${TEST_TOKEN}`,
};

// 场景1: 通知列表
export function testNoticeList() {
    const res = http.get(`${BASE_URL}/api/notice/list?page=1&size=20`, { headers });

    if (res.status !== 200) {
        console.log(`Notice Error ${res.status}: ${res.body}`);
    }

    check(res, {
        'notice list status 200': (r) => r.status === 200,
    });
    errorRate.add(res.status !== 200);
    responseTime.add(res.timings.duration);
}

// 场景2: 赛事列表
export function testCompList() {
    const res = http.get(`${BASE_URL}/api/comp/list?page=1&page_size=20`, { headers });

    if (res.status !== 200) {
        console.log(`Comp Error ${res.status}: ${res.body}`);
    }

    check(res, {
        'comp list status 200': (r) => r.status === 200,
    });
    errorRate.add(res.status !== 200);
    responseTime.add(res.timings.duration);
}

// 场景3: 奖项列表
export function testAwardList() {
    const res = http.get(`${BASE_URL}/api/award/list?page=1&size=20`, { headers });

    if (res.status !== 200) {
        console.log(`Award Error ${res.status}: ${res.body}`);
    }

    check(res, {
        'award list status 200': (r) => r.status === 200,
    });
    errorRate.add(res.status !== 200);
    responseTime.add(res.timings.duration);
}

// 场景4: 我的报名
export function testMyReg() {
    const res = http.get(`${BASE_URL}/api/reg/my-reg?page=1&size=20`, { headers });

    if (res.status !== 200) {
        console.log(`MyReg Error ${res.status}: ${res.body}`);
    }

    check(res, {
        'my reg status 200': (r) => r.status === 200,
    });
    errorRate.add(res.status !== 200);
    responseTime.add(res.timings.duration);
}

// 主场景配置 - 4K QPS 压测
export let options = {
    // 目标: 4K QPS = 4000 请求/秒
    // 80 VUs ≈ 2K QPS，所以需要 ~160 VUs 达到 4K
    scenarios: {
        default: {
            executor: 'constant-vus',
            vus: 80,
            duration: '60s',
        },
    },

    thresholds: {
        http_req_duration: ['p(95)<500'],
        http_req_failed: ['rate<0.01'],
    },
};

// 主函数
export default function() {
    // 随机选择一个接口进行测试, 模拟真实场景
    const rand = Math.floor(Math.random() * 4);
    switch (rand) {
        case 0:
            testNoticeList();
            break;
        case 1:
            testCompList();
            break;
        case 2:
            testAwardList();
            break;
        case 3:
            testMyReg();
            break;
    }

    // 模拟用户思考时间 (可选)
    sleep(0.01);  // 10ms 间隔
}