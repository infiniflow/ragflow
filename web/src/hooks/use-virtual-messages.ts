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

import { Virtualizer, useVirtualizer } from '@tanstack/react-virtual';
import { RefObject, useCallback, useEffect } from 'react';
import { IMessage } from '@/interfaces/database/chat';
import { buildMessageUuidWithRole } from '@/utils/chat';

// Until a message is measured, the virtualizer positions it with this
// estimate. A chat turn is usually a short question and a long answer, so a
// mid-sized value keeps the initial jump to the bottom close for both.
const EstimatedMessageHeight = 160;

// Losing the bottom by more than this means the user scrolled up on purpose.
const FollowBottomThreshold = 60;

export type MessageVirtualizer = Virtualizer<HTMLDivElement, HTMLDivElement>;

interface UseVirtualMessagesOptions {
  /** Changes when the displayed transcript is replaced, e.g. a conversation switch. */
  resetKey?: string;
}

export const useVirtualMessages = (
  messages: IMessage[] | undefined,
  containerRef: RefObject<HTMLDivElement>,
  { resetKey }: UseVirtualMessagesOptions = {},
) => {
  const virtualizer = useVirtualizer<HTMLDivElement, HTMLDivElement>({
    count: messages?.length ?? 0,
    getScrollElement: () => containerRef.current,
    getItemKey: (index) => {
      const message = messages?.[index];
      return message ? buildMessageUuidWithRole(message) : index;
    },
    estimateSize: () => EstimatedMessageHeight,
    overscan: 4,
    // A transcript is read from the bottom: keep the distance to the end
    // stable when messages are appended or resized, and follow new output
    // only while the user is still at the bottom.
    anchorTo: 'end',
    followOnAppend: true,
    scrollEndThreshold: FollowBottomThreshold,
  });

  const scrollToBottom = useCallback(() => {
    virtualizer.scrollToEnd();
  }, [virtualizer]);

  // The transcript pins itself to the bottom, so browser scroll anchoring is
  // pure interference while an answer re-wraps and re-measures.
  useEffect(() => {
    const container = containerRef.current;
    if (container) {
      container.style.overflowAnchor = 'none';
    }
  }, [containerRef]);

  // Appends are handled by followOnAppend. A replaced transcript (mount or
  // conversation switch) has no append to follow, so open it at the bottom.
  useEffect(() => {
    scrollToBottom();
  }, [resetKey, scrollToBottom]);

  return { virtualizer, scrollToBottom };
};
