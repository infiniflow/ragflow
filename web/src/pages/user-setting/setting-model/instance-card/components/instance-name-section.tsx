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

import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useTranslate } from '@/hooks/common-hooks';
import { Trash2 } from 'lucide-react';

export interface InstanceNameSectionProps {
  draftName: string;
  setDraftName: (name: string) => void;
  handleDelete: () => Promise<void>;
}

/**
 * The instance-name input section shown at the top of a draft (unsaved)
 * card. The input carries the destructive red border and is paired with
 * a delete icon. There is no inline Save button - the parent page's
 * top Save button drives persistence through the imperative ref API.
 * The helper text below explains the workflow.
 */
export function InstanceNameSection({
  draftName,
  setDraftName,
  handleDelete,
}: InstanceNameSectionProps) {
  const { t: tSetting } = useTranslate('setting');

  return (
    <div className="flex flex-col gap-1.5" data-testid="instance-name-section">
      <label
        htmlFor="instance-name-input"
        className="text-sm font-medium text-text-primary"
      >
        <span className="text-state-error mr-0.5">*</span>
        {tSetting('instanceName')}
      </label>
      <div className="flex items-center">
        <Input
          id="instance-name-input"
          value={draftName}
          onChange={(e) => setDraftName(e.target.value)}
          placeholder={tSetting('instanceNamePlaceholder')}
          // The input itself carries the red border. Persists while the
          // name is unsaved.
          className="flex-1"
          data-testid="instance-name-input"
        />
        <ConfirmDeleteDialog onOk={handleDelete}>
          <Button
            variant="delete"
            size="icon-sm"
            className="ml-2 shrink-0"
            aria-label={tSetting('deleteInstance')}
            data-testid="draft-delete"
          >
            <Trash2 className="size-4" />
          </Button>
        </ConfirmDeleteDialog>
      </div>
    </div>
  );
}
