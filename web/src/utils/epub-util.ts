/*
 *  Copyright 2026 The InfiFlow Authors. All Rights Reserved.
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

import JSZip from 'jszip';
import { decodeBlobText } from './file-util';

// EPUB (OCF spec) requires every XML document inside the archive to be
// UTF-8/UTF-16, and browsers only parse XML in those encodings. EPUBs
// exported by some tools still declare `encoding="GB2312"` with GBK
// bytes; browsers then fail XML parsing outright. Normalize every text
// entry to UTF-8 and rewrite the declaration so the browser can parse.
const TEXT_EXTENSIONS = ['.xhtml', '.html', '.htm', '.opf', '.ncx', '.xml'];

const isTextEntry = (name: string) =>
  TEXT_EXTENSIONS.some((ext) => name.toLowerCase().endsWith(ext));

export const normalizeEpubEncoding = async (
  data: ArrayBuffer,
): Promise<ArrayBuffer> => {
  const zip = await JSZip.loadAsync(data);
  let changed = false;

  for (const entry of Object.values(zip.files)) {
    if (entry.dir || !isTextEntry(entry.name)) {
      continue;
    }
    const buffer = await entry.async('arraybuffer');
    // decodeBlobText: strict UTF-8 first, GBK fallback for GB2312/GBK files
    const text = await decodeBlobText(buffer);
    const prolog = text.slice(0, 200);
    const normalized =
      /encoding\s*=\s*["']/.test(prolog) &&
      !/encoding\s*=\s*["']utf-?8["']/i.test(prolog)
        ? text.replace(
            /(<\?xml[^>]*?)encoding\s*=\s*["'][^"']*["']/i,
            '$1encoding="UTF-8"',
          )
        : text;
    if (normalized === text) {
      continue;
    }
    changed = true;
    zip.file(entry.name, normalized);
  }

  if (!changed) {
    return data;
  }
  return zip.generateAsync({ type: 'arraybuffer' });
};
