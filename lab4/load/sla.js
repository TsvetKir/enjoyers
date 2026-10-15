import http from 'k6/http';
import { Counter } from 'k6/metrics';

const orderErrors = new Counter('order_errors');

export const options = {
  scenarios: {
    orders: {
      executor: 'constant-arrival-rate',
      rate: 10,
      timeUnit: '1s',
      duration: __ENV.DURATION || '2m',
      preAllocatedVUs: 30,
      maxVUs: 30,
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<1000'],
    order_errors: ['count==0'],
    dropped_iterations: ['count==0'],
  },
};

export default function () {
  const response = http.post(
    'http://127.0.0.1:18080/order',
    JSON.stringify({ item: 'lab4-sla' }),
    {
      headers: { 'Content-Type': 'application/json' },
      timeout: '5s',
    },
  );

  orderErrors.add(response.status !== 201);
}