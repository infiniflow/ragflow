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
import { DynamicForm, DynamicFormRef } from '@/components/dynamic-form';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Input } from '@/components/ui/input';
import { useTranslate } from '@/hooks/common-hooks';
import { ListChevronsDownUp, ListChevronsUpDown, Trash2 } from 'lucide-react';
import {
  KeyboardEvent,
  useEffect,
  useRef,
  useState,
  type RefObject,
} from 'react';
import { useTranslation } from 'react-i18next';
import { DRAFT_INSTANCE_SENTINEL, SavedModeCardProps } from '../interface';
import { ModelsSection } from '../models-section';
import VerifyButton from '../verify-button';

/**
 * The saved (non-draft) variant of the provider instance card.
 *
 * Renders a Collapsible whose trigger shows a chevron toggle + the
 * instance name (double-click to rename) + delete button, and whose
 * content holds the form fields, the verify button, and (when
 * expanded) the per-instance models section.
 *
 * Auto-save has been removed; the parent's top Save button drives all
 * persistence through the imperative ref API.
 */
export function SavedModeCard({
  formFields,
  formDefaultValues,
  formRef,
  handleVerify,
  handleDelete,
  handleInstanceModelsEdited,
  providerName,
  instanceName,
  editedInstanceName,
  onRename,
  instance,
  instanceDetailsLoaded,
  modelInfoRef,
  draftName,
  open,
  setOpen,
}: SavedModeCardProps) {
  const { t } = useTranslation();
  const { t: tSetting } = useTranslate('setting');

  // Inline rename state: when true, the name turns into an editable
  // Input. The user double-clicks the name to enter rename mode,
  // commits on Enter/blur, cancels on Escape.
  const [renaming, setRenaming] = useState(false);
  const [renameValue, setRenameValue] = useState(editedInstanceName);
  const renameInputRef = useRef<HTMLInputElement>(null);

  // Sync the rename buffer when the displayed name changes externally
  // (e.g. after a successful save + refetch).
  useEffect(() => {
    setRenameValue(editedInstanceName);
  }, [editedInstanceName]);

  // Focus + select-all when entering rename mode.
  useEffect(() => {
    if (renaming) {
      const id = requestAnimationFrame(() => {
        renameInputRef.current?.focus();
        renameInputRef.current?.select();
      });
      return () => cancelAnimationFrame(id);
    }
  }, [renaming]);

  const startRename = () => {
    setRenameValue(editedInstanceName);
    setRenaming(true);
  };

  const commitRename = () => {
    setRenaming(false);
    const trimmed = renameValue.trim();
    if (trimmed && trimmed !== editedInstanceName) {
      onRename(trimmed);
    } else {
      setRenameValue(editedInstanceName);
    }
  };

  const cancelRename = () => {
    setRenaming(false);
    setRenameValue(editedInstanceName);
  };

  const handleRenameKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      commitRename();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      cancelRename();
    }
  };

  const displayName = editedInstanceName || instanceName;

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <div className="flex items-center gap-1 w-full mb-5">
          <div
            className="group w-[calc(100%-40px)] flex items-center flex-1 gap-2 px-2 py-1 cursor-pointer bg-bg-card rounded-md"
            data-testid="instance-name-row"
          >
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={
                open ? t('setting.hideModels') : t('setting.showMoreModels')
              }
              data-testid="instance-collapse"
            >
              {open ? (
                <ListChevronsDownUp className="size-4" />
              ) : (
                <ListChevronsUpDown className="size-4" />
              )}
            </Button>
            {renaming ? (
              <Input
                ref={renameInputRef}
                value={renameValue}
                onChange={(e) => setRenameValue(e.target.value)}
                onBlur={commitRename}
                onKeyDown={handleRenameKeyDown}
                onClick={(e) => e.stopPropagation()}
                onDoubleClick={(e) => e.stopPropagation()}
                className="text-sm font-medium h-7"
                data-testid="instance-name-rename-input"
              />
            ) : (
              <div
                className="text-sm font-medium truncate overflow-hidden flex-1 cursor-text"
                onDoubleClick={(e) => {
                  e.stopPropagation();
                  startRename();
                }}
                title={tSetting('editInstanceName')}
                data-testid="instance-name-static"
              >
                {draftName || displayName}
              </div>
            )}
          </div>
          <ConfirmDeleteDialog onOk={handleDelete}>
            <Button
              variant="delete"
              size="icon-sm"
              aria-label={tSetting('deleteInstance')}
              data-testid="instance-delete"
              onClick={(e: React.MouseEvent) => e.stopPropagation()}
            >
              <Trash2 className="size-4" />
            </Button>
          </ConfirmDeleteDialog>
        </div>
      </CollapsibleTrigger>
      <CollapsibleContent forceMount className="data-[state=closed]:hidden">
        <div className="pb-4 flex flex-col gap-4">
          <DynamicForm.Root
            key={`${providerName}-${instanceName}-false-${instanceDetailsLoaded ? 'loaded' : 'pending'}`}
            ref={formRef as RefObject<DynamicFormRef>}
            fields={formFields}
            onSubmit={() => undefined}
            defaultValues={formDefaultValues}
            labelClassName="font-normal"
          />

          <div className="pt-3">
            <VerifyButton
              onVerify={handleVerify}
              isAbsolute={false}
              formRef={formRef}
            />
          </div>

          {open && (
            <div className="pt-3">
              <ModelsSection
                providerName={providerName}
                instanceName={instanceName || DRAFT_INSTANCE_SENTINEL}
                instance={instance}
                hideActions={false}
                hideIfEmpty={false}
                instanceDetailsLoaded={instanceDetailsLoaded}
                getFormValues={() => formRef.current?.getValues?.() ?? {}}
                onInstanceModelsChange={(info) => {
                  modelInfoRef.current = info;
                }}
                onInstanceModelsEdited={handleInstanceModelsEdited}
              />
            </div>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
