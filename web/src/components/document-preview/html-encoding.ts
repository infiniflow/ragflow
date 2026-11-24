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

import { decodeBlobText } from '@/utils/file-util';

const HtmlEncodingScanBytes = 1024;
const ContentTypeCharset = /;\s*charset\s*=\s*["']?([\w.-]+)/i;
const HtmlAttribute =
  /([^\t\n\f\r /"'=<>]+)(?:[\t\n\f\r ]*=[\t\n\f\r ]*(?:"([^"]*)"|'([^']*)'|([^\t\n\f\r >]*)))?/g;

function getMetaEncoding(attributes: string): string | undefined {
  const values = new Map<string, string>();
  for (const [
    ,
    name,
    doubleQuoted,
    singleQuoted,
    unquoted,
  ] of attributes.matchAll(HtmlAttribute)) {
    const key = name.toLowerCase();
    if (!values.has(key)) {
      values.set(key, doubleQuoted ?? singleQuoted ?? unquoted ?? '');
    }
  }

  const legacyEncoding =
    values.get('http-equiv')?.toLowerCase() === 'content-type'
      ? values.get('content')?.match(ContentTypeCharset)?.[1]
      : undefined;
  const encoding = values.get('charset') ?? legacyEncoding;
  if (!encoding) return undefined;

  try {
    const name = new TextDecoder(encoding).encoding;
    // An HTML meta declaration cannot switch an ASCII-compatible scan to UTF-16.
    return name === 'utf-16le' || name === 'utf-16be' ? 'utf-8' : name;
  } catch {
    return undefined;
  }
}

function prescanHtmlEncoding(htmlPrefix: string): string | undefined {
  const tagStart = /<!--|<(\/?[a-z][^\t\n\f\r />]*|[!?])/gi;
  const tagBody = /(?:[^"'>]|"[^"]*"|'[^']*')*>/y;

  for (
    let tag = tagStart.exec(htmlPrefix);
    tag;
    tag = tagStart.exec(htmlPrefix)
  ) {
    if (tag[0] === '<!--') {
      const end = htmlPrefix.indexOf('-->', tag.index + 2);
      if (end === -1) return undefined;
      tagStart.lastIndex = end + 3;
      continue;
    }

    // Consume complete tags so markup inside a quoted attribute is skipped.
    tagBody.lastIndex = tagStart.lastIndex;
    const body = tagBody.exec(htmlPrefix);
    if (!body) return undefined;
    tagStart.lastIndex = tagBody.lastIndex;

    if (tag[1].toLowerCase() === 'meta') {
      const encoding = getMetaEncoding(body[0]);
      if (encoding) return encoding;
    }
  }
}

export async function decodeHtmlPreview(file: Blob): Promise<string> {
  const declaredEncoding =
    file.type.match(ContentTypeCharset)?.[1] ??
    prescanHtmlEncoding(await file.slice(0, HtmlEncodingScanBytes).text());
  return decodeBlobText(file, declaredEncoding);
}
