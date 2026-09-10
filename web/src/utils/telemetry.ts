import { Authorization } from '@/constants/authorization';
import { getAuthorization } from '@/utils/authorization-util';

const SESSION_KEY = 'ragflow.telemetry.session';
const NON_ACTIONABLE_ERROR_MESSAGES = new Set([
  'ResizeObserver loop completed with undelivered notifications.',
]);

function newTelemetryId() {
  try {
    return globalThis.crypto?.randomUUID?.() ?? '';
  } catch {
    return '';
  }
}

let interactionId = newTelemetryId();
let installed = false;
let reporting = false;
let lastReportAt = 0;

function sessionId() {
  try {
    const existing = sessionStorage.getItem(SESSION_KEY);
    if (existing) return existing;
    const created = newTelemetryId();
    if (created) sessionStorage.setItem(SESSION_KEY, created);
    return created;
  } catch {
    return '';
  }
}

export function getTelemetryHeaders(): Record<string, string> {
  const headers = {
    'X-Request-ID': newTelemetryId(),
    'X-Correlation-ID': interactionId,
    'X-Interaction-ID': interactionId,
    'X-Session-ID': sessionId(),
  };
  return Object.fromEntries(
    Object.entries(headers).filter(([, value]) => Boolean(value)),
  );
}

function rotateInteraction() {
  interactionId = newTelemetryId();
}

async function reportClientError(error: {
  name: string;
  message: string;
  line?: number;
  column?: number;
}) {
  if (NON_ACTIONABLE_ERROR_MESSAGES.has(error.message)) return;
  const now = Date.now();
  const authorization = getAuthorization();
  if (reporting || !authorization || now - lastReportAt < 10_000) return;
  reporting = true;
  lastReportAt = now;
  try {
    await fetch('/api/v1/system/client-errors', {
      method: 'POST',
      keepalive: true,
      headers: {
        'Content-Type': 'application/json',
        [Authorization]: authorization,
        ...getTelemetryHeaders(),
      },
      body: JSON.stringify({
        ...error,
        message: error.message.slice(0, 1000),
        route: location.pathname.slice(0, 512),
        browser: navigator.userAgent.slice(0, 256),
      }),
    });
  } finally {
    reporting = false;
  }
}

export function installClientTelemetry() {
  if (installed) return;
  installed = true;
  window.addEventListener('pointerdown', rotateInteraction, { capture: true });
  window.addEventListener('keydown', rotateInteraction, { capture: true });
  window.addEventListener('error', (event) => {
    void reportClientError({
      name: event.error?.name || 'WindowError',
      message: event.message || 'Unknown browser error',
      line: event.lineno,
      column: event.colno,
    });
  });
  window.addEventListener('unhandledrejection', (event) => {
    const reason = event.reason;
    void reportClientError({
      name: reason?.name || 'UnhandledRejection',
      message: reason?.message || String(reason || 'Unknown promise rejection'),
    });
  });
}
