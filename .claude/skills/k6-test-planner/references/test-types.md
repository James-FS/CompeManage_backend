# Test Type Selection Guide

Use this reference to help users pick the right test types for their goals. The mapping isn't
always 1:1 — a single user goal might require multiple test types, and a single test type can
serve multiple goals.

## Goal → Test Type Mapping

### "I want to make sure it works"
→ **Smoke test.** Low VU count (1-5), short duration (30s-1min). Validates that endpoints
respond correctly and return expected status codes. This is always a good starting point.

### "Can it handle our expected traffic?"
→ **Load test.** Ramp to the expected concurrent user count, hold for 5-15 minutes.
Set thresholds on p95 latency and error rate. Ask the user what "expected traffic" means
in concrete numbers — if they don't know, help them estimate from analytics, logs, or
reasonable assumptions.

### "Where does it break?"
→ **Stress test.** Start at expected load, ramp beyond it until errors appear or latency
degrades unacceptably. The goal is to find the ceiling, not to stay within safe bounds.
Useful for capacity planning and understanding failure modes.

### "Does it degrade over time?"
→ **Soak test.** Moderate load (50-80% of expected peak) sustained for 1-4 hours.
Catches memory leaks, connection pool exhaustion, log rotation issues, and other
time-dependent problems. Warn the user this takes a while.

### "How does it handle sudden bursts?"
→ **Spike test.** Rapid ramp from idle to high VU count, then back down. Tests auto-scaling,
queue behavior, and recovery. Common for event-driven systems (product launches, flash sales).

### "Does the UI actually work under load?"
→ **Browser test.** Uses k6's browser module to render pages in a real browser. Measures
Core Web Vitals (LCP, FID, CLS), page load time, and interaction responsiveness. More
expensive to run (fewer VUs possible), but catches rendering bottlenecks that protocol-level
tests miss.

### "I want to test the whole user journey"
→ **Combination.** Often: browser test for the critical path + load test for the API layer.
The browser test validates UX; the load test validates backend capacity. Don't try to do
heavy load with browser tests — they're resource-intensive.

### "I need to validate before deploying"
→ **Smoke test in CI/CD.** Quick, automated, runs on every deploy. Fails the pipeline if
response times or error rates exceed thresholds. Point the user toward k6's CI/CD integration
docs and suggest using `k6 cloud` or threshold-based exit codes.

## Common Patterns

### The Standard Suite
For most APIs, this covers the bases:
1. Smoke test (always first — no point load testing something that's broken)
2. Load test at expected traffic
3. Stress test to find the ceiling

### The Pre-Launch Suite
For systems about to face real traffic for the first time:
1. Smoke test
2. Load test at expected traffic
3. Stress test at 2-3x expected traffic
4. Spike test simulating launch-day surge
5. Soak test at expected traffic for 2+ hours

### The CI/CD Gate
For automated pipelines:
1. Smoke test only, with strict thresholds
2. Runs on every deploy, fails the build if thresholds are breached
3. Keep it under 60 seconds

### The Frontend Performance Audit
For UI-heavy applications:
1. Browser test for critical user flows (login, checkout, dashboard load)
2. Load test on the API endpoints the UI depends on
3. Compare against Core Web Vitals benchmarks

## Realistic VU Defaults

When the user doesn't specify VU counts, these are reasonable starting points. Always explain
your reasoning and let the user adjust.

| Scenario | VUs | Duration | Rationale |
|----------|-----|----------|-----------|
| Smoke | 1-5 | 30s-1min | Just checking it works |
| Small API (internal tool) | 10-50 | 5-10min | Typical internal service |
| Medium API (B2B SaaS) | 50-200 | 10-15min | Moderate concurrent usage |
| Large API (consumer-facing) | 200-1000 | 15-30min | High concurrent usage |
| Stress test multiplier | 2-3x load test | Until failure | Finding the ceiling |
| Soak test | 50-80% of load test | 1-4 hours | Sustained moderate load |
| Browser test | 1-10 | 2-5min | Browser tests are resource-heavy |

## Threshold Suggestions

When the user hasn't defined SLOs, suggest these as starting points:

| Metric | Typical Target | Aggressive Target |
|--------|---------------|-------------------|
| HTTP error rate | < 1% | < 0.1% |
| p95 response time | < 500ms | < 200ms |
| p99 response time | < 1000ms | < 500ms |
| Throughput | Stable (not declining) | > X req/s |

Always frame these as suggestions: "These are common industry targets — your actual SLOs should
reflect what your users expect."
