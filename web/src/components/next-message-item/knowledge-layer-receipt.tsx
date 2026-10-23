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

import { Collapse } from '@/components/collapse';
import { OntologyAssertionRow } from '@/components/ontology-assertion-row';
import { OntologyEvidenceBadge } from '@/components/ontology-evidence-badge';
import { IOntologyReceipt } from '@/interfaces/database/ontology-retrieval';
import { ontologyEvidenceHint } from '@/utils/ontology-evidence';
import { useTranslation } from 'react-i18next';

/**
 * What the knowledge layer contributed to one answer.
 *
 * It renders only when the turn read the layer, and it is deliberately collapsed:
 * a reader who trusts the answer needs one line that says how strong the
 * evidence under it was, and a reader checking it opens the rows and their
 * quotations. Nothing here is invented for the UI — the server ships the receipt
 * with the answer, so what is shown is what the retrieval actually did.
 */
export function KnowledgeLayerReceipt({
  receipt,
}: {
  receipt: IOntologyReceipt;
}) {
  const { t } = useTranslation();
  const assertions = receipt.assertions ?? [];
  const missingClasses = receipt.coverage?.missing_classes ?? [];

  const summary = (
    <span className="flex flex-wrap items-center gap-2 text-xs">
      <OntologyEvidenceBadge level={receipt.evidence_level} />
      <span>
        {t('chat.ontologyReceiptRows', {
          carried: receipt.rows_carried,
          total: receipt.rows_total,
        })}
      </span>
      <span className="text-text-disabled">
        {t('chat.ontologyPreviewModel', {
          name: receipt.model?.name || receipt.model?.id,
          version: receipt.model?.version ?? 0,
        })}
      </span>
      {receipt.unreviewed_count > 0 && (
        <span className="text-text-disabled">
          {t('chat.ontologyPreviewUnreviewed', {
            rows: receipt.unreviewed_count,
          })}
        </span>
      )}
      {receipt.citations.unresolvable > 0 && (
        <span className="text-text-disabled">
          {t('chat.ontologyReceiptUncitable', {
            rows: receipt.citations.unresolvable,
          })}
        </span>
      )}
    </span>
  );

  return (
    <div className="mt-2 rounded-lg border border-border-button bg-bg-card px-3 py-2">
      <Collapse title={summary}>
        <div className="space-y-2">
          <p className="text-xs text-text-secondary">
            {ontologyEvidenceHint(receipt.evidence_level, t)}
          </p>
          {receipt.degrade && (
            <p className="text-xs text-text-secondary">
              {t('chat.ontologyPreviewDegrade', {
                code: receipt.degrade.code,
                reason: receipt.degrade.reason,
              })}
            </p>
          )}
          {Boolean(receipt.applied_filter) && (
            <code className="block overflow-x-auto rounded-md bg-bg-input p-2 text-xs text-text-secondary">
              {JSON.stringify(receipt.applied_filter)}
            </code>
          )}
          {assertions.length > 0 ? (
            <ul className="space-y-2">
              {assertions.map((assertion, index) => (
                <OntologyAssertionRow
                  key={`${assertion.kind}-${assertion.type}-${assertion.name}-${assertion.from}-${assertion.to}-${index}`}
                  assertion={assertion}
                />
              ))}
            </ul>
          ) : (
            <p className="text-xs text-text-secondary">
              {t('chat.ontologyPreviewNoAssertion')}
            </p>
          )}
          {missingClasses.length > 0 && (
            <p className="text-xs text-text-disabled">
              {t('chat.ontologyPreviewMissingClasses', {
                classes: missingClasses.join(', '),
              })}
            </p>
          )}
        </div>
      </Collapse>
    </div>
  );
}
