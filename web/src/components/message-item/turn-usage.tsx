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

import { useTranslation } from 'react-i18next';
import { ITurnUsage } from '@/interfaces/database/chat';

/**
 * The one-line cost footer under an answer: tokens spent and how long the turn
 * took.
 *
 * It renders only when the turn actually reported a cost. The final SSE chunk
 * carries it, an answer restored from history does not — there is nowhere in the
 * message row to keep it — so showing a placeholder for those would advertise a
 * measurement that was never taken.
 */
export function TurnUsage({
  usage,
  elapsedMs,
}: {
  usage?: ITurnUsage;
  elapsedMs?: number;
}) {
  // `chat` is where the rest of the chat-setting wording lives; the footer is
  // part of the same surface.
  const { t } = useTranslation('translation', { keyPrefix: 'chat' });

  if (!usage || !usage.total_tokens) return null;

  const seconds = elapsedMs ? elapsedMs / 1000 : undefined;

  return (
    <div
      className="text-text-secondary text-xs mt-2 select-none"
      data-testid="turn-usage"
    >
      {t('tokensSpent', {
        total: usage.total_tokens,
        prompt: usage.prompt_tokens,
        completion: usage.completion_tokens,
      })}
      {seconds !== undefined && (
        <>
          {' · '}
          {t('turnElapsed', { seconds: seconds.toFixed(1) })}
        </>
      )}
    </div>
  );
}
