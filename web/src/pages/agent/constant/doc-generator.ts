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

export enum DocGeneratorOutputFormat {
  Pdf = 'pdf',
  Docx = 'docx',
  Txt = 'txt',
  Markdown = 'markdown',
  Html = 'html',
}

export const DocGeneratorFormatOptions = [
  { label: 'PDF', value: DocGeneratorOutputFormat.Pdf },
  { label: 'DOCX', value: DocGeneratorOutputFormat.Docx },
  { label: 'TXT', value: DocGeneratorOutputFormat.Txt },
  { label: 'Markdown', value: DocGeneratorOutputFormat.Markdown },
  { label: 'HTML', value: DocGeneratorOutputFormat.Html },
];

export const DocGeneratorFormatFeatures: Record<
  DocGeneratorOutputFormat,
  { decorations: boolean; timestamp: boolean }
> = {
  [DocGeneratorOutputFormat.Pdf]: { decorations: true, timestamp: true },
  [DocGeneratorOutputFormat.Docx]: { decorations: true, timestamp: true },
  [DocGeneratorOutputFormat.Txt]: { decorations: false, timestamp: true },
  [DocGeneratorOutputFormat.Markdown]: { decorations: false, timestamp: true },
  [DocGeneratorOutputFormat.Html]: { decorations: false, timestamp: true },
};
