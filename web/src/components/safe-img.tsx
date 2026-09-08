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

import type { ReactNode } from 'react';

interface SafeImgProps {
  src?: unknown;
  alt?: string;
  title?: string;
}

const SAFE_SRC_REGEXP = /^(https?:|\/|\.\/|\.\.\/|data:image\/)/i;

const buildImgTagString = (src?: unknown, alt?: string, title?: string) => {
  let tag = '<img';
  if (src != null) tag += ` src="${src}"`;
  if (alt != null) tag += ` alt="${alt}"`;
  if (title != null) tag += ` title="${title}"`;
  tag += '>';
  return tag;
};

/**
 * Drop all attributes except src/alt/title so event handlers (onerror/onload)
 * injected via rehypeRaw cannot fire. Used as a react-markdown `img` override.
 */
const isSafeImgSrc = (src: unknown): src is string =>
  typeof src === 'string' && SAFE_SRC_REGEXP.test(src.trim());

/**
 * Render an <img> safely for react-markdown overrides.
 *
 * - Safe src (http(s) / relative / `data:image`): render a real <img>,
 *   explicitly picking only src/alt/title so event handlers like `onerror` /
 *   `onload` that rehypeRaw may pass through cannot execute.
 * - Non-string or unsafe src (`javascript:` / `vbscript:` / unknown
 *   schemes): render the original `<img>` tag as a literal string. React
 *   escapes it, so the markup stays visible without executing scripts.
 */
export const SafeImg = ({ src, alt, title }: SafeImgProps): ReactNode => {
  if (!isSafeImgSrc(src)) return <>{buildImgTagString(src, alt, title)}</>;
  return <img src={src} alt={alt ?? ''} title={title} />;
};

export default SafeImg;
