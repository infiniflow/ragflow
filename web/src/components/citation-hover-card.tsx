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

import * as HoverCardPrimitive from '@radix-ui/react-hover-card';
import { PointerEvent, ReactNode, useCallback, useState } from 'react';
import { HoverCard, HoverCardContent, HoverCardTrigger } from './ui/hover-card';

type CitationHoverCardProps = {
  chip: ReactNode;
  // Omitted while the reference is still loading: the chip renders, but there
  // is nothing to open yet.
  content?: ReactNode;
};

// A citation chip whose card opens on hover and on tap.
//
// Radix HoverCard ignores touch pointers and cancels touchstart, so a tap on
// the chip produces neither a click nor focus and the card never opens on a
// phone. The card is therefore controlled: hover and focus still drive it
// through onOpenChange, and a touch pointerup opens it. Radix's dismissable
// layer closes it again on a tap outside or Escape. A scroll that starts on the
// chip ends in pointercancel, not pointerup, so it does not open the card.
export function CitationHoverCard({ chip, content }: CitationHoverCardProps) {
  const [open, setOpen] = useState(false);

  const handlePointerUp = useCallback((event: PointerEvent) => {
    if (event.pointerType === 'touch') {
      setOpen(true);
    }
  }, []);

  return (
    <HoverCard open={open} onOpenChange={setOpen}>
      <HoverCardTrigger onPointerUp={handlePointerUp}>{chip}</HoverCardTrigger>
      {content && (
        <HoverCardPrimitive.Portal>
          <HoverCardContent
            collisionPadding={12}
            className="max-w-[calc(100vw-1.5rem)] max-h-[var(--radix-hover-card-content-available-height)] md:max-w-3xl md:max-h-none"
          >
            {content}
          </HoverCardContent>
        </HoverCardPrimitive.Portal>
      )}
    </HoverCard>
  );
}
