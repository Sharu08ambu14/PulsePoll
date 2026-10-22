const configuredApiUrl = (import.meta.env.VITE_API_URL || '').replace(/\/+$/, '');
const isLocalDevelopment = ['localhost', '127.0.0.1', '::1'].includes(window.location.hostname);
const defaultApiUrl = isLocalDevelopment ? '/api' : 'https://pulsepoll-api-lhvw.onrender.com/api';

export const API_BASE = configuredApiUrl
  ? (configuredApiUrl.endsWith('/api') ? configuredApiUrl : `${configuredApiUrl}/api`)
  : defaultApiUrl;

async function request(path, options = {}) {
  const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) };
  let response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      ...options,
      headers,
    });
  } catch {
    const isLocalApi = /^https?:\/\/(localhost|127\.0\.0\.1|\[::1\])(:|\/)/.test(API_BASE);
    throw new Error(isLocalApi
      ? 'The local backend is not running at localhost:8080. Start MongoDB, Redis or Memurai, then run the backend in a second terminal.'
      : 'Cannot reach PulsePoll right now. The hosted server may be waking up; wait a minute and try again.');
  }

  const text = await response.text();
  let payload = {};
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = {};
    }
  }

  if (!response.ok) {
    const error = new Error(payload.error || `PulsePoll returned an error (${response.status}). Please retry in a minute.`);
    error.status = response.status;
    throw error;
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

export function listMyPolls(token) {
  return request('/polls/mine', { headers: { Authorization: `Bearer ${token}` } });
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
