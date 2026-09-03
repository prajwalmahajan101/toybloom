import http from 'k6/http';
import { check } from 'k6';
import { Rate } from 'k6/metrics';

// BASE_URL points at the running toybloom server (compose maps app→localhost:8080).
const BASE = __ENV.BASE_URL || 'http://localhost:8080';
const FILTER = `load-${__ENV.RUN_ID || 'local'}`;

// Representative workload: a short ramp to steady concurrency, then hold. The
// mix is ~50% Add / 50% Exists, the two hot user-facing paths whose p99 the DoD
// budgets at <200ms.
export const options = {
  scenarios: {
    steady: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '15s', target: 20 }, // ramp up
        { duration: '60s', target: 20 }, // hold steady
        { duration: '10s', target: 0 }, // ramp down
      ],
      gracefulRampDown: '5s',
    },
  },
  thresholds: {
    // The SLO gate: 99th-percentile request latency must stay under 200ms.
    http_req_duration: ['p(99)<200'],
    // Correctness under load: fewer than 0.1% of ops may error.
    op_errors: ['rate<0.001'],
  },
};

const errors = new Rate('op_errors');

// setup runs once: create the filter under test.
export function setup() {
  const res = http.post(`${BASE}/v1/filters`, JSON.stringify({ name: FILTER, n: 1000000, p: 0.01 }), {
    headers: { 'Content-Type': 'application/json' },
  });
  // 201 created, or 409 if a previous run left it — both are usable.
  check(res, { 'filter ready': (r) => r.status === 201 || r.status === 409 });
}

// Default VU loop: alternate Add and Exists on unique-ish keys.
export default function () {
  const val = `${__VU}-${__ITER}`;

  const add = http.post(`${BASE}/v1/filters/${FILTER}/items`, JSON.stringify({ value: val }), {
    headers: { 'Content-Type': 'application/json' },
    tags: { op: 'add' },
  });
  errors.add(add.status !== 200);
  check(add, { 'add ok': (r) => r.status === 200 });

  const exists = http.get(`${BASE}/v1/filters/${FILTER}/items/${encodeURIComponent(val)}`, {
    tags: { op: 'exists' },
  });
  errors.add(exists.status !== 200);
  // The value was just added on this VU, so it must be present — a wire-level
  // zero-false-negative assertion under concurrency.
  check(exists, {
    'exists ok': (r) => r.status === 200,
    'no false negative': (r) => r.json('data.exists') === true,
  });
}

// teardown runs once: drop the filter to keep runs repeatable.
export function teardown() {
  http.del(`${BASE}/v1/filters/${FILTER}`);
}
