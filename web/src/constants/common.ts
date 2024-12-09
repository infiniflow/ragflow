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
  'Indonesia',
  'Spanish',
  'Vietnamese',
  'Japanese',
];

export const LanguageMap = {
  English: 'English',
  Chinese: '简体中文',
  'Traditional Chinese': '繁體中文',
  Indonesia: 'Indonesia',
  Spanish: 'Español',
  Vietnamese: 'Tiếng việt',
  Japanese: '日本語',
};

export const LanguageTranslationMap = {
  English: 'en',
  Chinese: 'zh',
  'Traditional Chinese': 'zh-TRADITIONAL',
  Indonesia: 'id',
  Spanish: 'es',
  Vietnamese: 'vi',
  Japanese: 'ja',
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
export const ExceptiveType = ['xlsx', 'xls', 'pdf', 'docx', ...Images];

export const SupportedPreviewDocumentTypes = [...ExceptiveType];
//#endregion
