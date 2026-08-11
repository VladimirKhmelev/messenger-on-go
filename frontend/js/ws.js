export class WsClient {
  constructor({
    onMessageReceived,
    onNotifyPush,
    onHistory,
    onMessageSent,
    onPresenceChanged,
    onMessageUpdated,
    onError,
  }) {
    this.socket = null;
    this.pendingMessages = [];
    this.onMessageReceived = onMessageReceived;
    this.onNotifyPush = onNotifyPush;
    this.onHistory = onHistory;
    this.onMessageSent = onMessageSent;
    this.onPresenceChanged = onPresenceChanged;
    this.onMessageUpdated = onMessageUpdated;
    this.onError = onError;
  }

  connect(accessToken) {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${protocol}//${window.location.host}/ws?token=${encodeURIComponent(accessToken)}`;
    this.socket = new WebSocket(url);

    this.socket.addEventListener('open', () => {
      const queued = this.pendingMessages;
      this.pendingMessages = [];
      for (const payload of queued) {
        this.socket.send(JSON.stringify(payload));
      }
    });

    this.socket.addEventListener('message', (event) => {
      let msg;
      try {
        msg = JSON.parse(event.data);
      } catch {
        return;
      }
      this.dispatch(msg);
    });
  }

  dispatch(msg) {
    switch (msg.type) {
      case 'message_received':
        this.onMessageReceived?.({ chatId: msg.chat_id, message: normalizeMessage(msg.message) });
        break;
      case 'notify_push':
        this.onNotifyPush?.({ chatId: msg.chat_id, messageId: msg.message_id });
        break;
      case 'history':
        this.onHistory?.({ chatId: msg.chat_id, messages: (msg.messages || []).map(normalizeMessage) });
        break;
      case 'message_sent':
        this.onMessageSent?.({ messageId: msg.message_id });
        break;
      case 'presence_changed':
        this.onPresenceChanged?.({
          peerUserId: msg.peer_user_id,
          online: msg.online,
          lastSeenUnix: msg.last_seen_unix,
        });
        break;
      case 'message_updated':
        this.onMessageUpdated?.({
          chatId: msg.chat_id,
          messageId: msg.message_id,
          newText: msg.new_text ?? null,
          deleted: !!msg.deleted,
        });
        break;
      case 'error':
        this.onError?.({ error: msg.error });
        break;
    }
  }

  sendMessage(chatId, text) {
    this.send({ type: 'send_message', chat_id: chatId, text });
  }

  getHistory(chatId, limit = 50) {
    this.send({ type: 'get_history', chat_id: chatId, limit });
  }

  getPresence(peerUserId) {
    this.send({ type: 'get_presence', peer_user_id: peerUserId });
  }

  editMessage(chatId, messageId, text) {
    this.send({ type: 'edit_message', chat_id: chatId, message_id: messageId, text });
  }

  deleteMessageForAll(chatId, messageId) {
    this.send({ type: 'delete_message_for_all', chat_id: chatId, message_id: messageId });
  }

  deleteMessageForMe(chatId, messageId) {
    this.send({ type: 'delete_message_for_me', chat_id: chatId, message_id: messageId });
  }

  send(payload) {
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(payload));
    } else if (this.socket?.readyState === WebSocket.CONNECTING) {
      this.pendingMessages.push(payload);
    }
  }

  close() {
    this.socket?.close();
    this.socket = null;
    this.pendingMessages = [];
  }
}

function normalizeMessage(raw) {
  if (!raw) return null;
  return {
    messageId: raw.MessageID ?? raw.messageId,
    senderUserId: raw.SenderUserID ?? raw.senderUserId,
    text: raw.Text ?? raw.text,
    createdAtUnix: raw.CreatedAtUnix ?? raw.createdAtUnix,
    editedAtUnix: raw.EditedAtUnix ?? raw.editedAtUnix ?? 0,
    deleted: raw.Deleted ?? raw.deleted ?? false,
  };
}
