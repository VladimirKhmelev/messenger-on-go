import { pushApi } from './api.js';

function urlBase64ToUint8Array(base64) {
  const padding = '='.repeat((4 - (base64.length % 4)) % 4);
  const base64Safe = (base64 + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(base64Safe);
  const bytes = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i);
  return bytes;
}

export function isPushSupported() {
  return 'serviceWorker' in navigator && 'PushManager' in window && window.isSecureContext;
}

export async function registerServiceWorker() {
  if (!('serviceWorker' in navigator)) return null;
  await navigator.serviceWorker.register('/sw.js');
  return navigator.serviceWorker.ready;
}

export async function subscribeToPush(vapidPublicKeyBase64) {
  if (!isPushSupported()) {
    throw new Error('Push-уведомления не поддерживаются в этом браузере');
  }

  const permission = await Notification.requestPermission();
  if (permission !== 'granted') {
    throw new Error('Уведомления заблокированы в настройках браузера');
  }

  const registration = await registerServiceWorker();
  if (!registration) {
    throw new Error('Не удалось зарегистрировать service worker');
  }

  let subscription = await registration.pushManager.getSubscription();
  if (!subscription) {
    subscription = await registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(vapidPublicKeyBase64),
    });
  }

  const json = subscription.toJSON();
  await pushApi.saveSubscription(json.endpoint, json.keys.p256dh, json.keys.auth);
  return subscription;
}

export async function unsubscribeFromPush() {
  if (!('serviceWorker' in navigator)) return;

  const registration = await navigator.serviceWorker.getRegistration('/sw.js');
  if (!registration) return;

  const subscription = await registration.pushManager.getSubscription();
  if (!subscription) return;

  const endpoint = subscription.endpoint;
  await subscription.unsubscribe();
  await pushApi.deleteSubscription(endpoint);
}

export async function hasActivePushSubscription() {
  if (!('serviceWorker' in navigator)) return false;

  const registration = await navigator.serviceWorker.getRegistration('/sw.js');
  if (!registration) return false;

  const subscription = await registration.pushManager.getSubscription();
  return subscription !== null;
}
