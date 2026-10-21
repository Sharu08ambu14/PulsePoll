const configuredApiUrl = (import.meta.env.VITE_API_URL || '').replace(/\/+$/, '');

export const API_BASE = configuredApiUrl
  ? (configuredApiUrl.endsWith('/api') ? configuredApiUrl : `${configuredApiUrl}/api`)
  : 'http://localhost:8080/api';

async function request(path, options = {}) {
  const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) };
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
  });

  const text = await response.text();
  const payload = text ? JSON.parse(text) : {};

  if (!response.ok) {
    throw new Error(payload.error || 'Request failed');
  }

  return payload;
}

export function signup(data) {
  return request('/auth/signup', { method: 'POST', body: JSON.stringify(data) });
}

export function login(data) {
  return request('/auth/login', { method: 'POST', body: JSON.stringify(data) });
}

export function listPolls() {
  return request('/polls');
}

export function createPoll(data, token) {
  return request('/polls', {
    method: 'POST',
    body: JSON.stringify(data),
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });
}

export function getPoll(id) {
  return request(`/polls/${id}`);
}

export function submitVote(id, payload) {
  return request(`/polls/${id}/vote`, { method: 'POST', body: JSON.stringify(payload) });
}
