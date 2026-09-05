export const state = {
  view: 'login', // 'login' | 'app'
  authMode: 'login', // 'login' | 'register' | 'verify' | 'forgot-password' | 'reset-password' | 'github-passphrase'
  authError: '',
  authSuccess: '',
  authBusy: false,
  pendingVerifyEmail: '', // email awaiting a confirmation code, set when authMode === 'verify'
  pendingGitHubCode: '', // GitHub OAuth code awaiting the encryption passphrase, set when authMode === 'github-passphrase'
  tagCheck: null, // { tag, available, suggestedTag } — result of the last debounced tag-availability check

  currentUser: null, // { id, email, tag, displayName }

  chats: [],
  selectedChatId: null,
  searchQuery: '',
  foundUsers: [], // [{ id, tag, displayName }] — search results (tag prefix, 3+ chars) with no existing chat

  groupCreatorOpen: false, // true while the "new group" modal (member picker) is open
  groupCreatorSelected: [], // [{ id, tag, displayName }] — members picked so far in the modal
  groupCreatorName: '',
  groupCreatorError: '',
  groupCreatorBusy: false,
  groupCreatorQuery: '',
  groupCreatorFoundUsers: [],

  groupMembersOpen: false, // true while the "manage members" panel is open for the selected chat
  groupMembersBusy: false,
  groupMembersError: '',
  groupMembersAddQuery: '', // "add member" inline search field, admin-only
  groupMembersAddFoundUsers: [],
  groupMembersAvatarBusy: false,
  groupMembersAvatarError: '',
  draft: '',
  editingMessageId: null, // set while a message's inline edit field is open
  focusDraftOnRender: false, // one-shot flag: focus the composer input on the next conversation render
  scrollToBottomOnRender: false, // one-shot flag: force-scroll to the newest message on the next conversation render
  avatarPreview: null, // { userId, name } — set while the full-size avatar viewer is open

  toast: null, // { chatId, name, text }

  settingsOpen: false,
  settingsError: '',
  settingsBusy: false,
  settingsNameError: '',
  settingsNameBusy: false,
  settingsPasswordError: '',
  settingsPasswordSuccess: '',
  settingsPasswordBusy: false,
  settingsAvatarError: '',
  settingsAvatarBusy: false,
};

const listeners = {
  auth: [],
  sidebar: [],
  conversation: [],
  toast: [],
  settings: [],
  groupCreator: [],
  groupMembers: [],
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
