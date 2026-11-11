/** Model types accepted by the platform model API, in display order. */
export const PlatformModelTypes = [
  'chat',
  'embedding',
  'rerank',
  'vision',
  'asr',
  'tts',
  'ocr',
  'decision',
  'doc_parse',
] as const;

/** Types that can have a platform default (`doc_parse` is a capability). */
export const PlatformDefaultTypes = [
  'chat',
  'embedding',
  'vision',
  'asr',
  'rerank',
  'tts',
  'ocr',
  'decision',
] as const;

/** i18n keys for model type labels, shared with the tenant model settings. */
export const PlatformModelTypeLabelKeys: Record<string, string> = {
  chat: 'setting.modelTypes.chat',
  embedding: 'setting.modelTypes.embedding',
  rerank: 'setting.modelTypes.rerank',
  vision: 'setting.modelTypes.image2text',
  asr: 'setting.modelTypes.speech2text',
  tts: 'setting.modelTypes.tts',
  ocr: 'setting.modelTypes.ocr',
  decision: 'setting.modelTypes.decision',
  doc_parse: 'admin.platformModelsPage.docParse',
};

/**
 * Providers whose tenant form is a dedicated card rather than a provider
 * schema. Their credential shape cannot be entered through the shared
 * schema form, so the admin page does not offer them.
 */
export const UnsupportedPlatformProviders = new Set(['Bedrock']);
