let accessToken = null;

export function getAccessToken() {
  return accessToken;
}

export function setAccessToken(token) {
  accessToken = token;
}

export class ApiError extends Error {
  constructor(message, status) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function request(path, { method = 'GET', body, auth = true } = {}) {
  const headers = { 'Content-Type': 'application/json' };
  if (auth && accessToken) {
    headers['Authorization'] = `Bearer ${accessToken}`;
  }

  const response = await fetch(path, {
    method,
    headers,
    credentials: 'include',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  let data = null;
  const text = await response.text();
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }

  if (!response.ok) {
    const message = data?.message || data?.error || `Request failed (${response.status})`;
    throw new ApiError(message, response.status);
  }

  return data;
}


export const authApi = {
  register: (email, password, tag) =>
    request('/v1/auth/register', { method: 'POST', body: { email, password, tag }, auth: false }),

  login: (email, password) =>
    request('/v1/auth/login', { method: 'POST', body: { email, password }, auth: false }),

  loginWithGitHub: (code) =>
    request('/v1/auth/github', { method: 'POST', body: { code }, auth: false }),

  refresh: () => request('/v1/auth/refresh', { method: 'POST', body: {}, auth: false }),

  logout: () => request('/v1/auth/logout', { method: 'POST', body: {} }),

  verifyEmail: (email, code) =>
    request('/v1/auth/verify-email', { method: 'POST', body: { email, code }, auth: false }),

  requestPasswordReset: (email) =>
    request('/v1/auth/password-reset/request', { method: 'POST', body: { email }, auth: false }),

  resetPassword: (token, newPassword) =>
    request('/v1/auth/password-reset/confirm', {
      method: 'POST',
      body: { token, newPassword },
      auth: false,
    }),

  searchUsers: (query) => request(`/v1/users?query=${encodeURIComponent(query)}`),

  getUserByTag: (tag) => request(`/v1/users/by-tag/${encodeURIComponent(tag)}`),

  getUserByID: (userId) => request(`/v1/users/${encodeURIComponent(userId)}`),
};

export const chatApi = {
  listChats: () => request('/v1/chats'),

  createChat: (targetUserId) =>
    request('/v1/chats', { method: 'POST', body: { targetUserId } }),

  sendMessage: (chatId, text) =>
    request(`/v1/chats/${encodeURIComponent(chatId)}/messages`, { method: 'POST', body: { text } }),

  getHistory: (chatId, limit = 50) =>
    request(`/v1/chats/${encodeURIComponent(chatId)}/messages?limit=${limit}`),
};

export async function refreshAccessToken() {
  try {
    const data = await authApi.refresh();
    if (data?.accessToken) {
      setAccessToken(data.accessToken);
      return true;
    }
    return false;
  } catch {
    return false;
  }
}

export function currentUserIdFromToken() {
  if (!accessToken) return null;
  try {
    const payload = accessToken.split('.')[1];
    const json = atob(payload.replace(/-/g, '+').replace(/_/g, '/'));
    return JSON.parse(json).user_id ?? null;
  } catch {
    return null;
  }
}
