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

import DOMPurify from 'dompurify';

export const scrollToBottom = (element: HTMLElement) => {
  element.scrollTo(0, element.scrollHeight);
};

/**
 * Sanitize HTML and render any <img> elements as escaped text strings
 * instead of loading them as images. Prevents image-based tracking /
 * data exfiltration via external image URLs and layout disruption, while
 * keeping the img markup visible to the user as literal text.
 *
 * Event-handler XSS (onerror) is already stripped by DOMPurify's default
 * config; this additionally neutralizes the image element itself.
 */
export const sanitizeHtmlWithImagesAsText = (html: string): string => {
  const node = DOMPurify.sanitize(html, { RETURN_DOM: true }) as HTMLElement;
  node.querySelectorAll('img').forEach((img) => {
    img.replaceWith(document.createTextNode(img.outerHTML));
  });
  return node.innerHTML;
};
