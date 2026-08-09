export const state = {
  view: 'login', // 'login' | 'app'
  authMode: 'login', // 'login' | 'register' | 'verify'
  authError: '',
  authBusy: false,
  pendingVerifyEmail: '', // email awaiting a confirmation code, set when authMode === 'verify'

  currentUser: null, // { id, email, tag }

  chats: [], // { id, peer: {id, email, tag}, messages: [], online }
  selectedChatId: null,
  searchQuery: '',
  foundUser: null, // { id, tag } — result of an exact-tag lookup when search matches no existing chat
  draft: '',

  toast: null, // { chatId, name, text }
};

const listeners = {
  auth: [],
  sidebar: [],
  conversation: [],
  toast: [],
};

export function onZone(zone, fn) {
  listeners[zone].push(fn);
}

export function notify(zone) {
  for (const fn of listeners[zone]) fn();
}

export function notifyAll() {
  for (const zone of Object.keys(listeners)) notify(zone);
}
