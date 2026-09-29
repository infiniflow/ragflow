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
  [FileType.Video]: ['mp4', 'avi', 'mkv'],
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
  rmvb: 'rmvb',
  wav: 'wav',
  html: 'html',
  json: 'json',
};
