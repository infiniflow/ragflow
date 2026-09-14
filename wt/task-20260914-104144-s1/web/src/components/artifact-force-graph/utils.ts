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

import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';

export const defaultMapNodeToValue = <TNode extends IArtifactGraphEntity>(
  node: TNode,
): TNode => node;

export const getBaseLinkColor = (element?: HTMLElement | null): string => {
  if (typeof window === 'undefined' || !element) {
    return '#b2b5b7';
  }
  return window
    .getComputedStyle(element)
    .getPropertyValue('--border-default')
    .trim();
};

export const withAlpha = (color: string, alpha: number): string => {
  if (color.length === 7 && color.startsWith('#')) {
    return (
      color +
      Math.round(alpha * 255)
        .toString(16)
        .padStart(2, '0')
    );
  }
  if (color.startsWith('rgb(')) {
    return `rgba(${color.slice(4, -1)}, ${alpha})`;
  }
  return color;
};
