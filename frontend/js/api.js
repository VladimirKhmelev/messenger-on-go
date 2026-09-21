import { errorMessages } from './i18n.js';

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


export function translateApiError(err) {
  if (!(err instanceof ApiError)) return null;
  return errorMessages()[err.message] || err.message;
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
  register: (email, password, tag, displayName, publicKey, wrappedPrivateKey, keyWrapSalt) =>
    request('/v1/auth/register', {
      method: 'POST',
      body: { email, password, tag, displayName, publicKey, wrappedPrivateKey, keyWrapSalt },
      auth: false,
    }),

  login: (email, password) =>
    request('/v1/auth/login', { method: 'POST', body: { email, password }, auth: false }),

  loginWithGitHub: (code, publicKey, wrappedPrivateKey, keyWrapSalt) =>
    request('/v1/auth/github', {
      method: 'POST',
      body: { code, publicKey, wrappedPrivateKey, keyWrapSalt },
      auth: false,
    }),

  refresh: () => request('/v1/auth/refresh', { method: 'POST', body: {}, auth: false }),

  logout: () => request('/v1/auth/logout', { method: 'POST', body: {} }),

  verifyEmail: (email, code) =>
    request('/v1/auth/verify-email', { method: 'POST', body: { email, code }, auth: false }),

  requestPasswordReset: (email) =>
    request('/v1/auth/password-reset/request', { method: 'POST', body: { email }, auth: false }),

  resetPassword: (token, newPassword, publicKey, wrappedPrivateKey, keyWrapSalt) =>
    request('/v1/auth/password-reset/confirm', {
      method: 'POST',
      body: { token, newPassword, publicKey, wrappedPrivateKey, keyWrapSalt },
      auth: false,
    }),

  searchUsers: (query) => request(`/v1/users?query=${encodeURIComponent(query)}`),

  getUserByTag: (tag) => request(`/v1/users/by-tag/${encodeURIComponent(tag)}`),

  getUserByID: (userId) => request(`/v1/users/${encodeURIComponent(userId)}`),

  checkTagAvailable: (tag) =>
    request(`/v1/users/tag-available/${encodeURIComponent(tag)}`, { auth: false }),

  updateTag: (tag) => request('/v1/users/me/tag', { method: 'PATCH', body: { tag } }),

  updateDisplayName: (displayName) =>
    request('/v1/users/me/display-name', { method: 'PATCH', body: { displayName } }),

  changePassword: (oldPassword, newPassword, wrappedPrivateKey, keyWrapSalt) =>
    request('/v1/users/me/password', {
      method: 'POST',
      body: { oldPassword, newPassword, wrappedPrivateKey, keyWrapSalt },
    }),

  deleteAccount: (password) =>
    request('/v1/users/me/delete', { method: 'POST', body: { password } }),

  getPublicKey: (userId) => request(`/v1/users/${encodeURIComponent(userId)}/public-key`),

  getWrappedPrivateKey: () => request('/v1/users/me/wrapped-private-key'),

  uploadAvatar: async (file) => {
    const response = await fetch('/v1/users/me/avatar', {
      method: 'POST',
      headers: { 'Content-Type': file.type, Authorization: `Bearer ${accessToken}` },
      credentials: 'include',
      body: file,
    });

    if (!response.ok) {
      const text = await response.text();
      throw new ApiError(text || `Request failed (${response.status})`, response.status);
    }
  },
};

export const chatApi = {
  listChats: () => request('/v1/chats'),

  createChat: (targetUserId, encryptedChatKey, wrappedForPublicKey) =>
    request('/v1/chats', { method: 'POST', body: { targetUserId, encryptedChatKey, wrappedForPublicKey } }),

  createGroupChat: (name, targetUserIds, encryptedChatKey, wrappedForPublicKey) =>
    request('/v1/chats/group', {
      method: 'POST',
      body: { name, targetUserIds, encryptedChatKey, wrappedForPublicKey },
    }),

  addMember: (chatId, userId, encryptedChatKey, wrappedForPublicKey) =>
    request(`/v1/chats/${encodeURIComponent(chatId)}/members`, {
      method: 'POST',
      body: { userId, encryptedChatKey, wrappedForPublicKey },
    }),

  removeMember: (chatId, userId) =>
    request(`/v1/chats/${encodeURIComponent(chatId)}/members/${encodeURIComponent(userId)}`, {
      method: 'DELETE',
    }),

  setMemberRole: (chatId, userId, role) =>
    request(`/v1/chats/${encodeURIComponent(chatId)}/members/${encodeURIComponent(userId)}/role`, {
      method: 'PUT',
      body: { role },
    }),

  leaveChat: (chatId) => request(`/v1/chats/${encodeURIComponent(chatId)}/leave`, { method: 'POST', body: {} }),

  deleteGroupChat: (chatId) => request(`/v1/chats/${encodeURIComponent(chatId)}`, { method: 'DELETE' }),

  listChatMembers: (chatId) => request(`/v1/chats/${encodeURIComponent(chatId)}/members`),

  getChatKey: (chatId) => request(`/v1/chats/${encodeURIComponent(chatId)}/key`),

  listChatKeys: (chatId) => request(`/v1/chats/${encodeURIComponent(chatId)}/keys`),

  updateChatKey: (chatId, userId, encryptedChatKey, wrappedForPublicKey) =>
    request(`/v1/chats/${encodeURIComponent(chatId)}/keys/${encodeURIComponent(userId)}`, {
      method: 'PUT',
      body: { encryptedChatKey, wrappedForPublicKey },
    }),

  sendMessage: (chatId, text) =>
    request(`/v1/chats/${encodeURIComponent(chatId)}/messages`, { method: 'POST', body: { text } }),

  getHistory: (chatId, limit = 50) =>
    request(`/v1/chats/${encodeURIComponent(chatId)}/messages?limit=${limit}`),

  uploadGroupAvatar: async (chatId, file) => {
    const response = await fetch(`/v1/chats/${encodeURIComponent(chatId)}/avatar`, {
      method: 'POST',
      headers: { 'Content-Type': file.type, Authorization: `Bearer ${accessToken}` },
      credentials: 'include',
      body: file,
    });

    if (!response.ok) {
      const text = await response.text();
      throw new ApiError(text || `Request failed (${response.status})`, response.status);
    }
  },

  blockUser: (userId) =>
    request(`/v1/chats/blocked/${encodeURIComponent(userId)}`, { method: 'POST', body: {} }),

  unblockUser: (userId) =>
    request(`/v1/chats/blocked/${encodeURIComponent(userId)}`, { method: 'DELETE' }),

  listBlockedUsers: () => request('/v1/chats/blocked'),

  reportMessage: (messageId, category, comment) =>
    request(`/v1/chat/messages/${encodeURIComponent(messageId)}/report`, {
      method: 'POST',
      body: { category, comment },
    }),
};

export const mediaApi = {
  requestUpload: (chatId, contentType, sizeBytes) =>
    request('/v1/media/uploads', {
      method: 'POST',
      body: { chatId, contentType, sizeBytes },
    }),

  confirmUpload: (uploadId) =>
    request(`/v1/media/uploads/${encodeURIComponent(uploadId)}/confirm`, { method: 'POST', body: {} }),

  getDownloadUrl: (mediaId) => request(`/v1/media/${encodeURIComponent(mediaId)}/download-url`),

  // Presigned URLs go straight to MinIO, not through our own /v1 API — no
  // auth header, no JSON envelope, just the raw (already-encrypted) bytes.
  putEncrypted: async (uploadUrl, encryptedBlob) => {
    const response = await fetch(uploadUrl, { method: 'PUT', body: encryptedBlob });
    if (!response.ok) {
      throw new ApiError(`Upload failed (${response.status})`, response.status);
    }
  },

  fetchEncrypted: async (downloadUrl) => {
    const response = await fetch(downloadUrl);
    if (!response.ok) {
      throw new ApiError(`Download failed (${response.status})`, response.status);
    }
    return response.arrayBuffer();
  },
};

export const pushApi = {
  saveSubscription: (endpoint, p256dhKey, authKey) =>
    request('/v1/users/me/push-subscription', {
      method: 'POST',
      body: { endpoint, p256dhKey, authKey },
    }),

  deleteSubscription: (endpoint) =>
    request('/v1/users/me/push-subscription/delete', {
      method: 'POST',
      body: { endpoint },
    }),
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
