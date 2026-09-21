import ru from './locales/ru.js';
import en from './locales/en.js';

const LOCALES = { ru, en };
const STORAGE_KEY = 'wisp-locale';
const DEFAULT_LOCALE = 'ru';

export function getLocale() {
  const stored = localStorage.getItem(STORAGE_KEY);
  return stored && LOCALES[stored] ? stored : DEFAULT_LOCALE;
}

export function setLocale(locale) {
  if (!LOCALES[locale]) return;
  localStorage.setItem(STORAGE_KEY, locale);
}

// Looks up a dot-separated key (e.g. "auth.login.title") in the active
// locale, falling back to Russian (the app's original hardcoded language)
// if a key is missing there — never falls back to the key itself, so a
// missing translation is loud (visible "[[key]]" in the UI) rather than
// silently showing raw English/Russian in the wrong context.
export function t(key, params) {
  const value = lookup(LOCALES[getLocale()], key) ?? lookup(ru, key);
  if (value === undefined) {
    console.error(`i18n: missing key "${key}"`);
    return `[[${key}]]`;
  }
  return params ? interpolate(value, params) : value;
}

// Server error messages are themselves the lookup keys (full English
// sentences, dots and all), so they can't go through t()'s dot-notation
// path — callers index this map directly.
export function errorMessages() {
  return { ...ru.errors, ...LOCALES[getLocale()].errors };
}

function lookup(dict, key) {
  return key.split('.').reduce((obj, part) => obj?.[part], dict);
}

function interpolate(str, params) {
  return str.replace(/\{(\w+)\}/g, (match, name) => (name in params ? String(params[name]) : match));
}
