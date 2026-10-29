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
import { useTranslate } from '@/hooks/common-hooks';
import { ListCheck, Loader2 } from 'lucide-react';
import { MouseEvent, useCallback } from 'react';

interface InstanceSaveButtonProps {
  /** Persist the instance this button belongs to. */
  onSave: () => void;
  /** True while the save request is in flight. */
  saving: boolean;
}

/**
 * Header Save button of an instance card. Persists only its own
 * instance; the click must not bubble into the surrounding collapsible
 * trigger, which would toggle the card open/closed.
 */
export function InstanceSaveButton({
  onSave,
  saving,
}: InstanceSaveButtonProps) {
  const { t } = useTranslate('setting');

  const handleClick = useCallback(
    (e: MouseEvent<HTMLButtonElement>) => {
      e.stopPropagation();
      onSave();
    },
    [onSave],
  );

  return (
    <Button
      size="sm"
      onClick={handleClick}
      disabled={saving}
      data-testid="instance-save"
      className="gap-1.5"
    >
      {saving ? (
        <Loader2 className="size-4 animate-spin" />
      ) : (
        <ListCheck className="size-4" />
      )}
      {t('save')}
    </Button>
  );
}
