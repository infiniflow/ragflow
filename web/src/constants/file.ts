/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

export enum FileType {
  PDF = 'pdf',
  Spreadsheet = 'spreadsheet',
  Image = 'image',
  Email = 'email',
  TextMarkdown = 'markdown',
  Code = 'text&code',
  Html = 'html',
  Doc = 'doc',
  Docx = 'docx',
  PowerPoint = 'slides',
  Video = 'video',
  Audio = 'audio',
}

export const FileTypeSuffixMap = {
  [FileType.PDF]: ['pdf'],
  [FileType.Spreadsheet]: ['xls', 'xlsx', 'csv'],
  [FileType.Image]: ['jpg', 'jpeg', 'png', 'gif'],
  [FileType.Email]: ['eml', 'msg'],
  [FileType.TextMarkdown]: ['md', 'markdown', 'mdx'],
  [FileType.Code]: [
    'txt',
    'py',
    'js',
    'java',
    'c',
    'cpp',
    'h',
    'php',
    'go',
    'ts',
    'sh',
    'cs',
    'kt',
    'sql',
  ],
  [FileType.Html]: ['htm', 'html'],
  [FileType.Doc]: ['doc'],
  [FileType.Docx]: ['docx'],
  [FileType.PowerPoint]: ['pptx', 'ppt'],
  [FileType.Video]: [
    // Mirrors utility.videoExtensions (internal/utility/file.go): upload
    // admission, runtime dispatch and the parser-family mapping read one set.
    'mp4',
    'avi',
    'mkv',
    'mov',
    'webm',
    'flv',
    'mpeg',
    'mpg',
    'wmv',
    '3gp',
    '3gpp',
    'rm',
    'rmvb',
    'asf',
    'dat',
    'asx',
    'wvx',
    'mpe',
    'mpa',
  ],
  [FileType.Audio]: [
    'da',
    'wave',
    'wav',
    'mp3',
    'aac',
    'flac',
    'ogg',
    'aiff',
    'au',
    'midi',
    'wma',
    'realaudio',
    'vqf',
    'oggvorbis',
    'ape',
  ],
};

export const FileIconMap = {
  doc: 'doc',
  docx: 'doc',
  pdf: 'pdf',
  xls: 'excel',
  xlsx: 'excel',
  ppt: 'ppt',
  pptx: 'ppt',
  jpg: 'jpg',
  jpeg: 'jpg',
  png: 'png',
  txt: 'text',
  csv: 'excel',
  md: 'md',
  mdx: 'md',
  mp4: 'mp4',
  avi: 'avi',
  mkv: 'mkv',
  mov: 'mp4',
  webm: 'mp4',
  flv: 'mp4',
  wmv: 'mp4',
  mpeg: 'mp4',
  mpg: 'mp4',
  '3gp': 'mp4',
  '3gpp': 'mp4',
  rm: 'mp4',
  rmvb: 'rmvb',
  asf: 'mp4',
  dat: 'mp4',
  asx: 'mp4',
  wvx: 'mp4',
  mpe: 'mp4',
  mpa: 'mp4',
  wav: 'wav',
  html: 'html',
  json: 'json',
};
