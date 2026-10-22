export const CanvasBackgroundGlobalKey = 'sys.canvas_background';

export type CanvasBackgroundSetting = {
  mode: 'default' | 'color' | 'image';
  color?: string;
  image?: string;
};

export const CanvasBackgroundPresets = [
  '#0b0f14',
  '#111827',
  '#1e293b',
  '#1f2937',
  '#0f2744',
  '#f8fafc',
  '#e5e7eb',
  '#fef3c7',
] as const;

const MaxImageCharacters = 1_800_000;

function safeCanvasColor(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined;
  return /^#[0-9a-f]{6}$/i.test(value) ? value : undefined;
}

function safeCanvasImage(value: unknown): string | undefined {
  if (typeof value !== 'string' || value.length > MaxImageCharacters) {
    return undefined;
  }
  return /^data:image\/jpeg;base64,[a-z0-9+/]+={0,2}$/i.test(value)
    ? value
    : undefined;
}

export function parseCanvasBackground(value: unknown): CanvasBackgroundSetting {
  if (!value || typeof value !== 'object') return { mode: 'default' };
  const raw = value as Partial<CanvasBackgroundSetting>;
  const color = safeCanvasColor(raw.color);
  if (raw.mode === 'color' && color) {
    return { mode: 'color', color };
  }
  const image = safeCanvasImage(raw.image);
  if (raw.mode === 'image' && image) {
    return { mode: 'image', image, color };
  }
  return { mode: 'default' };
}

export function canvasBackgroundFromGlobals(
  globals?: object | null,
): CanvasBackgroundSetting {
  if (!globals) return { mode: 'default' };
  return parseCanvasBackground(
    (globals as Record<string, unknown>)[CanvasBackgroundGlobalKey],
  );
}

function canvasBackgroundStorageKey(agentId: string): string {
  return `${CanvasBackgroundGlobalKey}:${agentId}`;
}

export function readStoredCanvasBackground(
  agentId?: string,
): CanvasBackgroundSetting | undefined {
  if (!agentId) return undefined;
  try {
    const raw = sessionStorage.getItem(canvasBackgroundStorageKey(agentId));
    if (!raw) return undefined;
    return parseCanvasBackground(JSON.parse(raw) as unknown);
  } catch {
    return undefined;
  }
}

export function writeStoredCanvasBackground(
  agentId: string,
  setting: CanvasBackgroundSetting,
): void {
  try {
    sessionStorage.setItem(
      canvasBackgroundStorageKey(agentId),
      JSON.stringify(setting),
    );
  } catch {
    // Storage can be blocked or full. The server copy remains.
  }
}

function backdropColor(color?: string): string {
  const chosen = safeCanvasColor(color);
  if (chosen) return chosen;
  if (typeof document === 'undefined') return '#000000';
  const channels = getComputedStyle(document.documentElement)
    .getPropertyValue('--bg-canvas')
    .trim();
  if (/^\d{1,3}\s+\d{1,3}\s+\d{1,3}$/.test(channels)) {
    return `rgb(${channels.split(/\s+/).join(', ')})`;
  }
  return '#000000';
}

export async function readCanvasBackgroundImage(
  file: File,
  color?: string,
): Promise<string> {
  const bitmap = await createImageBitmap(file);
  const maxEdge = 1600;
  const scale = Math.min(1, maxEdge / Math.max(bitmap.width, bitmap.height));
  const width = Math.max(1, Math.round(bitmap.width * scale));
  const height = Math.max(1, Math.round(bitmap.height * scale));
  const canvas = document.createElement('canvas');
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext('2d');
  if (!context) {
    bitmap.close();
    throw new Error('canvas');
  }
  context.fillStyle = backdropColor(color);
  context.fillRect(0, 0, width, height);
  context.drawImage(bitmap, 0, 0, width, height);
  bitmap.close();
  const dataUrl = canvas.toDataURL('image/jpeg', 0.82);
  if (dataUrl.length > MaxImageCharacters) {
    throw new Error('too-large');
  }
  return dataUrl;
}
