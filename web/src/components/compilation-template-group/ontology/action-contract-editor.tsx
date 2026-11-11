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

'use client';

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  ACTION_CONTRACT_FIELD_ORDER,
  ActionContractFieldName,
  IModelAction,
} from '@/interfaces/database/compilation-model';
import { useUpdateActionContract } from '@/hooks/use-ontology-action-request';
import message from '@/components/ui/message';
import { ReactNode, useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

/** Reads a field's current value into the editor's string form. */
function fieldToText(action: IModelAction, name: ActionContractFieldName): string {
  const field = action.contract?.fields?.[name];
  if (!field || field.status === 'missing') {
    return '';
  }
  if (field.status === 'explicit_none' && field.value === undefined) {
    return '';
  }
  if (Array.isArray(field.value)) {
    return field.value.map((item) => String(item)).join('\n');
  }
  if (typeof field.value === 'boolean') {
    return field.value ? 'true' : 'false';
  }
  return field.value === undefined || field.value === null
    ? ''
    : String(field.value);
}

/**
 * Manual contract editing.
 *
 * Two behaviours here are load-bearing rather than cosmetic:
 *
 * 1. Only the fields the operator actually changed are submitted. Writing a
 *    value deletes that field's quote on the server, so submitting untouched
 *    fields would silently strip the document's own wording from fields nobody
 *    meant to touch — turning citable values into "entered manually" ones.
 *
 * 2. The reason is required. This dialog changes what the system is allowed to
 *    do; the audit row records why, not only what.
 */
export function ActionContractEditor({
  templateId,
  action,
  trigger,
  onSaved,
}: {
  templateId: string;
  action: IModelAction;
  trigger: ReactNode;
  onSaved?: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState('');
  const { loading, updateContract } = useUpdateActionContract();

  const initial = useMemo(() => {
    const out: Record<string, string> = {};
    ACTION_CONTRACT_FIELD_ORDER.forEach((name) => {
      out[name] = fieldToText(action, name);
    });
    return out;
  }, [action]);

  const [draft, setDraft] = useState<Record<string, string>>(initial);

  const handleOpenChange = useCallback(
    (next: boolean) => {
      setOpen(next);
      if (next) {
        // Reset on open: a cancelled edit must not come back as a draft when the
        // dialog is reopened for the same row.
        setDraft(initial);
        setReason('');
      }
    },
    [initial],
  );

  const changed = useMemo(() => {
    const out: Record<string, unknown> = {};
    ACTION_CONTRACT_FIELD_ORDER.forEach((name) => {
      const next = (draft[name] ?? '').trim();
      if (next === (initial[name] ?? '').trim()) {
        return;
      }
      if (next === '') {
        // null clears the field: "we were wrong, the document says nothing".
        out[name] = null;
        return;
      }
      const lines = next
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean);
      // Lists keep their line structure; everything else is one string.
      out[name] = lines.length > 1 ? lines : next;
    });
    return out;
  }, [draft, initial]);

  const handleSubmit = useCallback(async () => {
    const reasonText = reason.trim();
    if (!reasonText) {
      message.error(
        t('knowledgeCompilation.ontologyInspect.actionContractReasonRequired', {
          defaultValue: 'State why the contract is being changed.',
        }),
      );
      return;
    }
    if (Object.keys(changed).length === 0) {
      message.error(
        t('knowledgeCompilation.ontologyInspect.actionContractNoChange', {
          defaultValue: 'Nothing changed.',
        }),
      );
      return;
    }
    try {
      await updateContract({
        id: templateId,
        actionId: action.id,
        datasetId: action.dataset_id,
        contract: changed,
        reason: reasonText,
      });
      message.success(
        t('knowledgeCompilation.ontologyInspect.actionContractSaved', {
          defaultValue: 'Contract updated.',
        }),
      );
      setOpen(false);
      onSaved?.();
    } catch (error) {
      // The server's rejection carries the field names; showing it as-is keeps
      // one explanation instead of a generic "failed".
      message.error((error as Error)?.message);
    }
  }, [reason, changed, updateContract, templateId, action, t, onSaved]);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t('knowledgeCompilation.ontologyInspect.actionContractEditTitle', {
              defaultValue: 'Edit contract · {{name}}',
              name: action.name,
            })}
          </DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-3 text-xs">
          <p className="text-text-secondary">
            {t('knowledgeCompilation.ontologyInspect.actionContractEditIntro', {
              defaultValue:
                'A value entered here replaces the document’s wording for that field: it becomes authoritative for execution, but no source text supports it, so it can no longer be quoted or cited. Leave a field empty to clear it.',
            })}
          </p>

          {ACTION_CONTRACT_FIELD_ORDER.map((name) => {
            const field = action.contract?.fields?.[name];
            return (
              <div key={name} className="flex flex-col gap-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Label htmlFor={`contract-${name}`} className="text-xs">
                    {t(
                      `knowledgeCompilation.ontologyInspect.actionContractField.${name}`,
                      { defaultValue: name },
                    )}
                  </Label>
                  {field?.provenance === 'document' && field.quote && (
                    <span className="text-text-disabled">
                      {t(
                        'knowledgeCompilation.ontologyInspect.actionContractEditFromDoc',
                        { defaultValue: 'read from the document' },
                      )}
                    </span>
                  )}
                </div>
                <Textarea
                  id={`contract-${name}`}
                  value={draft[name] ?? ''}
                  rows={2}
                  className="text-xs"
                  onChange={(e) =>
                    setDraft((prev) => ({ ...prev, [name]: e.target.value }))
                  }
                />
                {field?.quote && (
                  <blockquote className="border-l-2 border-border-button pl-2 text-text-secondary">
                    {field.quote}
                  </blockquote>
                )}
              </div>
            );
          })}

          <div className="flex flex-col gap-1">
            <Label htmlFor="contract-reason" className="text-xs">
              {t('knowledgeCompilation.ontologyInspect.actionContractReason', {
                defaultValue: 'Reason (required, kept in the audit trail)',
              })}
            </Label>
            <Textarea
              id="contract-reason"
              value={reason}
              rows={2}
              className="text-xs"
              onChange={(e) => setReason(e.target.value)}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            {t('common.cancel')}
          </Button>
          <Button loading={loading} onClick={handleSubmit}>
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
