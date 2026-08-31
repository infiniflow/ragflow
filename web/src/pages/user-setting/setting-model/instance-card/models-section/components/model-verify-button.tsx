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
import { Check, Loader2, RefreshCcw, TriangleAlert } from 'lucide-react';
import { ModelVerifyButtonProps, VerifyStatus } from '../interface';

/** Icon + color for a single verify status. */
const VERIFY_ICON: Record<
  VerifyStatus,
  { icon: React.ReactNode; className: string }
> = {
  idle: {
    icon: <RefreshCcw className="size-3" />,
    className: 'text-text-secondary hover:bg-bg-input hover:text-text-primary',
  },
  loading: {
    icon: <Loader2 className="size-4 animate-spin" />,
    className: 'text-text-secondary cursor-wait',
  },
  success: {
    icon: <Check className="size-4" />,
    className: 'text-state-success',
  },
  error: {
    icon: <TriangleAlert className="size-4" />,
    className: 'text-state-warning',
  },
};

/** Per-model verify button. */
export function ModelVerifyButton({
  status,
  onVerify,
  modelName,
}: ModelVerifyButtonProps) {
  const cfg = VERIFY_ICON[status];
  return (
    <button
      type="button"
      className={cn(
        'size-6 flex items-center justify-center rounded-md transition-colors',
        cfg.className,
      )}
      onClick={onVerify}
      disabled={status === 'loading'}
      aria-label={`Verify ${modelName}`}
    >
      {cfg.icon}
    </button>
  );
}
