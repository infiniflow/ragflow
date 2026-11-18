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

import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { RefreshCcw } from 'lucide-react';
import { memo } from 'react';
import { useTranslation } from 'react-i18next';

interface RetryableErrorProps {
  /** What went wrong; falls back to a generic request-failed text. */
  message?: string;
  /** Re-sends the request that failed. */
  onRetry: () => void;
  /** Disables the button while the retried request is in flight. */
  retrying?: boolean;
  className?: string;
}

/** An inline error with a Retry button next to it. */
function RetryableError({
  message,
  onRetry,
  retrying,
  className,
}: RetryableErrorProps) {
  const { t } = useTranslation();

  return (
    <div
      role="alert"
      className={cn(
        'flex items-center gap-3 rounded-lg bg-bg-card p-3 text-state-error',
        className,
      )}
    >
      <span className="min-w-0 flex-1 break-words">
        {message || t('message.requestError')}
      </span>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onRetry}
        disabled={retrying}
      >
        <RefreshCcw className={cn(retrying && 'animate-spin-reverse')} />
        {t('common.retry')}
      </Button>
    </div>
  );
}

export default memo(RetryableError);
