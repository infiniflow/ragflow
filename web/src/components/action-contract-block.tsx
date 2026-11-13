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

import { Badge } from '@/components/ui/badge';
import {
  ACTION_CONTRACT_FIELD_ORDER,
  ActionContractFieldName,
  IActionContractField,
} from '@/interfaces/database/compilation-model';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';

/**
 * An action's execution contract, as the fields this model declares.
 *
 * The block exists to make one distinction impossible to miss: "the document
 * does not say" is not "no approval is needed". A reader who sees an empty
 * authorization row and reads it as unrestricted is exactly how an action runs
 * on a misread document, so `missing` says so in words rather than being an
 * empty slot.
 *
 * A manual value is marked too, for the opposite reason: it is authoritative for
 * execution but no chunk says it, so it may not be cited as evidence.
 */
export function ActionContractBlock({
  fields,
  missing,
  declared,
  required,
  className,
}: {
  fields?: Record<string, IActionContractField>;
  missing?: string[];
  declared?: number;
  /**
   * The declaration the contract was judged against. A template may narrow it, and
   * a field the model was never asked for is not owed — listing it as missing is
   * how a complete contract comes to look incomplete.
   */
  required?: ActionContractFieldName[];
  className?: string;
}) {
  const { t } = useTranslation();
  if (!fields) {
    return null;
  }
  const requiredFields = required ?? ACTION_CONTRACT_FIELD_ORDER;
  const requiredSet = new Set(requiredFields);
  const missingSet = new Set(missing ?? []);
  const total = requiredFields.length;
  const filled = declared ?? total - missingSet.size;

  return (
    <div
      className={cn(
        'rounded border border-border-button p-2 space-y-1 text-xs',
        className,
      )}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">
          {t('knowledgeCompilation.ontologyInspect.actionContractTitle', {
            defaultValue: 'Execution contract',
          })}
        </span>
        <span className="text-text-secondary">{`${filled}/${total}`}</span>
      </div>

      {ACTION_CONTRACT_FIELD_ORDER.map((name) => {
        const field = fields[name];
        if (!field) {
          return null;
        }
        // A field outside the declaration is not owed, so it is not reported as
        // missing. A value that exists is still shown: it is either an operator's
        // entry or a value extracted before the template narrowed, and hiding it
        // would hide the only place it can be read.
        if (!requiredSet.has(name) && field.status === 'missing') {
          return null;
        }
        return (
          <ContractFieldRow
            key={name}
            name={name}
            field={field}
            isMissing={missingSet.has(name)}
          />
        );
      })}

      <p className="text-text-disabled pt-1">
        {t('knowledgeCompilation.ontologyInspect.actionContractMissingNote', {
          defaultValue:
            'A missing field means the document does not state it — not that no restriction applies.',
        })}
      </p>
    </div>
  );
}

function ContractFieldRow({
  name,
  field,
  isMissing,
}: {
  name: ActionContractFieldName;
  field: IActionContractField;
  isMissing: boolean;
}) {
  const { t } = useTranslation();
  const label = t(
    `knowledgeCompilation.ontologyInspect.actionContractField.${name}`,
    {
      defaultValue: name,
    },
  );
  const missing = isMissing || field.status === 'missing';

  return (
    <div className="flex flex-col gap-0.5 border-t border-border-button pt-1 first:border-t-0 first:pt-0">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-text-secondary">{label}</span>
        {missing && (
          <span className="text-text-disabled">
            {t('knowledgeCompilation.ontologyInspect.actionContractMissing', {
              defaultValue: 'not stated by the document',
            })}
          </span>
        )}
        {!missing && field.status === 'explicit_none' && (
          <Badge variant="secondary">
            {t(
              'knowledgeCompilation.ontologyInspect.actionContractExplicitNone',
              {
                defaultValue: 'explicitly none',
              },
            )}
          </Badge>
        )}
        {!missing && field.provenance === 'manual' && (
          <Badge
            variant="secondary"
            className="text-state-warning border-state-warning"
          >
            {/* Not a warning about the value: a warning that it cannot be cited. */}
            {t('knowledgeCompilation.ontologyInspect.actionContractManual', {
              defaultValue: 'no source quote',
            })}
          </Badge>
        )}
      </div>

      {!missing && field.value !== undefined && field.value !== null && (
        <span className="text-text-primary">{renderValue(field.value)}</span>
      )}

      {field.quote && (
        <blockquote className="border-l-2 border-border-button pl-2 text-text-secondary">
          {field.quote}
        </blockquote>
      )}
    </div>
  );
}

/** Renders the three value shapes the column stores without guessing at more. */
function renderValue(value: unknown): string {
  if (typeof value === 'boolean') {
    return value ? 'true' : 'false';
  }
  if (Array.isArray(value)) {
    return value.map((item) => String(item)).join(' · ');
  }
  return String(value);
}
