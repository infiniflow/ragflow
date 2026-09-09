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

import { Children, cloneElement, Fragment, isValidElement } from 'react';
import type { ReactElement, ReactNode } from 'react';

const URL_REGEX = /(https?:\/\/[^\s<>)"{}|^`[\]]+)/gi;
const TRAILING_PUNCTUATION_REGEX = /[.,!?;:'。，！？；：）】》」』’”]+$/u;
const LINK_CLASS_NAME = 'text-buttonBlueText underline hover:opacity-80';

interface LinkifyTextProps {
  children: ReactNode;
  className?: string;
}

function linkifyString(children: string) {
  const parts: ReactNode[] = [];
  let lastIndex = 0;
  let match;

  while ((match = URL_REGEX.exec(children)) !== null) {
    if (match.index > lastIndex) {
      parts.push(
        <span key={`text-${lastIndex}`}>
          {children.slice(lastIndex, match.index)}
        </span>,
      );
    }

    const matchedUrl = match[0];
    const url = matchedUrl.replace(TRAILING_PUNCTUATION_REGEX, '');
    parts.push(
      <a
        key={`link-${match.index}`}
        href={url}
        target="_blank"
        rel="noopener noreferrer"
        className={LINK_CLASS_NAME}
        onClick={(e) => e.stopPropagation()}
      >
        {url}
      </a>,
    );

    const trailingPunctuation = matchedUrl.slice(url.length);
    if (trailingPunctuation) {
      parts.push(trailingPunctuation);
    }

    lastIndex = match.index + matchedUrl.length;
  }

  if (lastIndex < children.length) {
    parts.push(
      <span key={`text-${lastIndex}`}>{children.slice(lastIndex)}</span>,
    );
  }

  return parts;
}

function linkifyHtml(html: string) {
  let isInsideLink = false;

  return html
    .split(/(<[^>]+>)/g)
    .map((part) => {
      if (part.startsWith('<')) {
        if (/^<a(?:\s|>)/i.test(part)) isInsideLink = true;
        if (/^<\/a\s*>/i.test(part)) isInsideLink = false;
        return part;
      }

      if (isInsideLink) return part;

      return part.replace(URL_REGEX, (matchedUrl) => {
        const url = matchedUrl.replace(TRAILING_PUNCTUATION_REGEX, '');
        const trailingPunctuation = matchedUrl.slice(url.length);
        return `<a href="${url}" target="_blank" rel="noopener noreferrer" class="${LINK_CLASS_NAME}">${url}</a>${trailingPunctuation}`;
      });
    })
    .join('');
}

function linkifyNode(node: ReactNode): ReactNode {
  if (typeof node === 'string') {
    return linkifyString(node);
  }

  if (
    !isValidElement(node) ||
    (typeof node.type === 'string' && node.type === 'a')
  ) {
    return node;
  }

  const element = node as ReactElement<{
    children?: ReactNode;
    dangerouslySetInnerHTML?: { __html: string };
  }>;
  if (element.props.dangerouslySetInnerHTML) {
    return cloneElement(element, {
      dangerouslySetInnerHTML: {
        __html: linkifyHtml(element.props.dangerouslySetInnerHTML.__html),
      },
    });
  }

  if (element.props.children === undefined) {
    return node;
  }

  return cloneElement(
    element,
    undefined,
    Children.map(element.props.children, linkifyNode),
  );
}

export function LinkifyText({ children, className }: LinkifyTextProps) {
  const content = Children.map(children, linkifyNode);
  return className ? (
    <span className={className}>{content}</span>
  ) : (
    <Fragment>{content}</Fragment>
  );
}
