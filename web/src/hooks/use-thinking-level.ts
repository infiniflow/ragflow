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

import storage from '@/utils/authorization-util';
import { useSyncExternalStore } from 'react';

// Values mirror the strings persisted under the `thinkingLevel` localStorage
// key; they double as the reasoning level sent with each message ('0' naive,
// '5' agentic).
export const ThinkingMode = { Naive: '0', Agentic: '5' } as const;
export type ThinkingMode = (typeof ThinkingMode)[keyof typeof ThinkingMode];

const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

// Persists the level, then notifies same-tab subscribers (e.g. the chat
// settings panel) so a Type change in the message input re-renders them.
export function setThinkingLevel(mode: ThinkingMode) {
  storage.setThinkingLevel(mode);
  listeners.forEach((notify) => notify());
}

export function useThinkingLevel(): ThinkingMode {
  // getThinkingLevel already sanitizes unknown persisted values to Naive.
  return useSyncExternalStore(
    subscribe,
    storage.getThinkingLevel,
  ) as ThinkingMode;
}
