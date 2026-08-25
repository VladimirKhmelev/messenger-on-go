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

const ERROR_TRANSLATIONS = {
  'invalid email': 'Некорректный email',
  'invalid tag': 'Тег должен быть на латинице в нижнем регистре, от 3 до 20 символов',
  'invalid display name': 'Некорректное имя',
  'password does not meet complexity requirements': 'Пароль не соответствует требованиям',
  'email already registered': 'Этот email уже зарегистрирован',
  'tag already taken': 'Тег уже занят',
  'user not found': 'Пользователь не найден',
  'invalid or expired token': 'Ссылка недействительна или истекла',
  'invalid email or password': 'Неверный email или пароль',
  'search query must be at least 3 characters': 'Введите минимум 3 символа для поиска',
  'too many login attempts, try again later': 'Слишком много попыток входа, попробуйте позже',
  'invalid or expired verification code': 'Код неверный или устарел',
  'email not verified': 'Email не подтверждён',
  'oauth provider account has no verified email': 'У аккаунта OAuth-провайдера нет подтверждённого email',
  'new password must be different from the current password': 'Новый пароль должен отличаться от текущего',
  'avatar not found': 'Аватар не найден',
  'avatar must be a JPEG, PNG, GIF, or WebP image': 'Аватар должен быть в формате JPEG, PNG, GIF или WebP',
  'avatar must be smaller than 2MB': 'Аватар должен быть меньше 2 МБ',
  'invalid public key': 'Не удалось сгенерировать ключ шифрования, попробуйте снова',
  'user has not uploaded an encryption public key': 'У пользователя нет ключа шифрования',
  'chat not found': 'Чат не найден',
  'user is not a member of this chat': 'Вы не участник этого чата',
  'message body must not be empty': 'Сообщение не может быть пустым',
  'private chat between these users already exists': 'Чат с этим пользователем уже существует',
  'cannot create a chat with yourself': 'Нельзя создать чат с самим собой',
  'target user not found': 'Пользователь не найден',
  'message not found': 'Сообщение не найдено',
  'message has been deleted': 'Сообщение удалено',
  'only the sender can edit or delete this message for everyone': 'Только автор может редактировать или удалить сообщение для всех',
  'message does not belong to this chat': 'Сообщение не принадлежит этому чату',
  'message body exceeds maximum allowed size': 'Сообщение слишком длинное',
  'too many messages sent, slow down': 'Слишком много сообщений, помедленнее',
};

export function translateApiError(err) {
  if (!(err instanceof ApiError)) return null;
  return ERROR_TRANSLATIONS[err.message] || err.message;
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
