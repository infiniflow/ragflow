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

import { cn } from '@/lib/utils';
import { useRef } from 'react';

import { useX6Graph } from './hooks/use-x6-graph';
import { type TimelineX6GraphProps } from './types';

export function TimelineX6Graph({
  data,
  show = true,
  onNodeClick,
}: TimelineX6GraphProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  useX6Graph(containerRef, data, onNodeClick);

  return (
    <div
      ref={containerRef}
      className={cn('w-full h-full min-h-0', !show && 'hidden')}
    />
  );
}

export default TimelineX6Graph;
