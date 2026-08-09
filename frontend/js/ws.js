export class WsClient {
  constructor({ onMessageReceived, onNotifyPush, onHistory, onMessageSent, onError }) {
    this.socket = null;
    this.onMessageReceived = onMessageReceived;
    this.onNotifyPush = onNotifyPush;
    this.onHistory = onHistory;
    this.onMessageSent = onMessageSent;
    this.onError = onError;
  }

  connect(accessToken) {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${protocol}//${window.location.host}/ws?token=${encodeURIComponent(accessToken)}`;
    this.socket = new WebSocket(url);

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
        this.onHistory?.({ messages: (msg.messages || []).map(normalizeMessage) });
        break;
      case 'message_sent':
        this.onMessageSent?.({ messageId: msg.message_id });
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

  send(payload) {
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(payload));
    }
  }

  close() {
    this.socket?.close();
    this.socket = null;
  }
}

function normalizeMessage(raw) {
  if (!raw) return null;
  return {
    messageId: raw.MessageID ?? raw.messageId,
    senderUserId: raw.SenderUserID ?? raw.senderUserId,
    text: raw.Text ?? raw.text,
    createdAtUnix: raw.CreatedAtUnix ?? raw.createdAtUnix,
  };
}
