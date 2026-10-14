import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    orders: {
      executor: 'constant-arrival-rate',
      rate: 10,
      timeUnit: '1s',
      duration: '15m',
      preAllocatedVUs: 10,
      maxVUs: 30,
    },
  },
};

export default function () {
  const requests = [18081, 18082, 18083].map((port) => ({
    method: 'POST',
    url: `http://127.0.0.1:${port}/order`,
    body: JSON.stringify({ item: 'lab4-baseline' }),
    params: {
      headers: { 'Content-Type': 'application/json' },
      timeout: '5s',
    },
  }));

  const responses = http.batch(requests);
  for (const response of responses) {
    check(response, {
      'order created': (r) => r.status === 201,
    });
  }
}