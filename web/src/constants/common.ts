export const fileIconMap = {
  aep: 'aep.svg',
  ai: 'ai.svg',
  avi: 'avi.svg',
  css: 'css.svg',
  csv: 'csv.svg',
  dmg: 'dmg.svg',
  doc: 'doc.svg',
  docx: 'docx.svg',
  eps: 'eps.svg',
  exe: 'exe.svg',
  fig: 'fig.svg',
  gif: 'gif.svg',
  html: 'html.svg',
  indd: 'indd.svg',
  java: 'java.svg',
  jpeg: 'jpeg.svg',
  jpg: 'jpg.svg',
  js: 'js.svg',
  json: 'json.svg',
  md: 'md.svg',
  mdx: 'mdx.svg',
  mkv: 'mkv.svg',
  mp3: 'mp3.svg',
  mp4: 'mp4.svg',
  mpeg: 'mpeg.svg',
  pdf: 'pdf.svg',
  png: 'png.svg',
  ppt: 'ppt.svg',
  pptx: 'pptx.svg',
  psd: 'psd.svg',
  rss: 'rss.svg',
  sql: 'sql.svg',
  svg: 'svg.svg',
  tiff: 'tiff.svg',
  txt: 'txt.svg',
  wav: 'wav.svg',
  webp: 'webp.svg',
  xls: 'xls.svg',
  xlsx: 'xlsx.svg',
  xml: 'xml.svg',
};

export const LanguageList = [
  'English',
  'Chinese',
  'Traditional Chinese',
  'Russian',
  'Indonesia',
  'Spanish',
  'Vietnamese',
  'Japanese',
  'Portuguese BR',
  'German',
  'French',
  'Italian',
];
export const LanguageMap = {
  English: 'English',
  Chinese: '简体中文',
  'Traditional Chinese': '繁體中文',
  Russian: 'Русский',
  Indonesia: 'Indonesia',
  Spanish: 'Español',
  Vietnamese: 'Tiếng việt',
  Japanese: '日本語',
  'Portuguese BR': 'Português BR',
  German: 'German',
  French: 'Français',
  Italian: 'Italiano',
};

export enum LanguageAbbreviation {
  En = 'en',
  Zh = 'zh',
  ZhTraditional = 'zh-TRADITIONAL',
  Ru = 'ru',
  Id = 'id',
  Ja = 'ja',
  Es = 'es',
  Vi = 'vi',
  PtBr = 'pt-BR',
  De = 'de',
  Fr = 'fr',
  It = 'it',
}

export const LanguageAbbreviationMap = {
  [LanguageAbbreviation.En]: 'English',
  [LanguageAbbreviation.Zh]: '简体中文',
  [LanguageAbbreviation.ZhTraditional]: '繁體中文',
  [LanguageAbbreviation.Ru]: 'Русский',
  [LanguageAbbreviation.Id]: 'Indonesia',
  [LanguageAbbreviation.Es]: 'Español',
  [LanguageAbbreviation.Vi]: 'Tiếng việt',
  [LanguageAbbreviation.Ja]: '日本語',
  [LanguageAbbreviation.PtBr]: 'Português BR',
  [LanguageAbbreviation.De]: 'Deutsch',
  [LanguageAbbreviation.Fr]: 'Français',
  [LanguageAbbreviation.It]: 'Italiano',
};

export const LanguageTranslationMap = {
  English: 'en',
  Chinese: 'zh',
  'Traditional Chinese': 'zh-TRADITIONAL',
  Russian: 'ru',
  Indonesian: 'id',
  Spanish: 'es',
  Vietnamese: 'vi',
  Japanese: 'ja',
  Korean: 'ko',
  'Portuguese BR': 'pt-br',
  German: 'de',
  French: 'fr',
  Italian: 'it',
  Tamil: 'ta',
  Telugu: 'te',
  Kannada: 'ka',
  Thai: 'th',
  Greek: 'el',
  Hindi: 'hi',
  Ukrainian: 'uk',
};

export enum FileMimeType {
  Bmp = 'image/bmp',
  Csv = 'text/csv',
  Odt = 'application/vnd.oasis.opendocument.text',
  Doc = 'application/msword',
  Docx = 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  Gif = 'image/gif',
  Htm = 'text/htm',
  Html = 'text/html',
  Jpg = 'image/jpg',
  Jpeg = 'image/jpeg',
  Pdf = 'application/pdf',
  Png = 'image/png',
  Ppt = 'application/vnd.ms-powerpoint',
  Pptx = 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
  Tiff = 'image/tiff',
  Txt = 'text/plain',
  Xls = 'application/vnd.ms-excel',
  Xlsx = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  Mp4 = 'video/mp4',
  Json = 'application/json',
  Md = 'text/markdown',
  Mdx = 'text/markdown',
}

export const Domain = 'demo.ragflow.io';

//#region file preview
export const Images = [
  'jpg',
  'jpeg',
  'png',
  'gif',
  'bmp',
  'tif',
  'tiff',
  'webp',
  // 'svg',
  'ico',
];

// Without FileViewer
export const ExceptiveType = [
  'xlsx',
  'xls',
  'pdf',
  'docx',
  'md',
  'mdx',
  ...Images,
];

export const SupportedPreviewDocumentTypes = [...ExceptiveType];
//#endregion

export enum Platform {
  RAGFlow = 'RAGFlow',
  Dify = 'Dify',
  FastGPT = 'FastGPT',
  Coze = 'Coze',
}

export enum ThemeEnum {
  Dark = 'dark',
  Light = 'light',
  System = 'system',
}
