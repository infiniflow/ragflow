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

const URL_REGEX = /(https?:\/\/[^\s<>)"{}|^`[\]]+)/gi;

interface LinkifyTextProps {
  children: ReactNode;
  className?: string;
}

export function LinkifyText({ children, className }: LinkifyTextProps) {
  if (typeof children !== 'string') {
    return <span className={className}>{children}</span>;
  }

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

    const url = match[0];
    parts.push(
      <a
        key={`link-${match.index}`}
        href={url}
        target="_blank"
        rel="noopener noreferrer"
        className="text-buttonBlueText underline hover:opacity-80"
        onClick={(e) => e.stopPropagation()}
      >
        {url}
      </a>,
    );

    lastIndex = match.index + url.length;
  }

  if (lastIndex < children.length) {
    parts.push(
      <span key={`text-${lastIndex}`}>{children.slice(lastIndex)}</span>,
    );
  }

  return <span className={className}>{parts}</span>;
}
