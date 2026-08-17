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

import { FileIconMap } from '@/constants/file';
import { cn } from '@/lib/utils';
import { getExtension } from '@/utils/document-util';
import { CSSProperties } from 'react';
import SvgIcon from './svg-icon';

type IconFontType = {
  name: string;
  className?: string;
  style?: CSSProperties;
};

export const IconFont = ({ name, className, style }: IconFontType) => (
  <svg className={cn('size-4', className)} style={style}>
    <use xlinkHref={`#icon-${name}`} />
  </svg>
);

export function IconFontFill({
  name,
  className,
  isFill = true,
}: IconFontType & { isFill?: boolean }) {
  return (
    <svg
      className={cn('size-4', className)}
      style={{ fill: isFill ? 'currentColor' : '' }}
    >
      <use xlinkHref={`#icon-${name}`} />
    </svg>
  );
}

export function FileIcon({
  name,
  className,
  type,
}: IconFontType & { type?: string }) {
  const isFolder = type === 'folder';
  const isSkills = type === 'skills';
  if (isSkills) {
    return (
      <span className={cn('size-4', className)}>
        <SvgIcon name="home-icon/skills" width={16} height={16} />
      </span>
    );
  }
  return (
    <span className={cn('size-4', className)}>
      <IconFont
        name={isFolder ? 'file-sub' : FileIconMap[getExtension(name)]}
      ></IconFont>
    </span>
  );
}
