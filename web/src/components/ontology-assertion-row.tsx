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
import { IOntologyAssertion } from '@/interfaces/database/ontology-retrieval';
import { useTranslation } from 'react-i18next';

/** Mirrors service.AssertionReviewUnreviewed: a row nobody has judged yet. */
const OntologyAssertionUnreviewed = 'unreviewed';

const OntologyAssertionKind = {
  Entity: 'entity',
  Relation: 'relation',
} as const;

export function ontologyAssertionTitle(assertion: IOntologyAssertion) {
  if (assertion.kind === OntologyAssertionKind.Relation) {
    return `${assertion.from} → ${assertion.to} (${assertion.type})`;
  }
  return `${assertion.type} · ${assertion.name}`;
}

/**
 * One compiled assertion, as both the preview and an answer's receipt show it.
 *
 * The quotation is the row's evidence and is rendered verbatim: it is the part
 * of an assertion that may be read as a citation, and it is quoted text rather
 * than a re-worded claim on purpose.
 */
export function OntologyAssertionRow({
  assertion,
}: {
  assertion: IOntologyAssertion;
}) {
  const { t } = useTranslation();
  const chunkCount = assertion.chunk_ids?.length ?? 0;
  const attributes = Object.entries(assertion.attributes ?? {});

  return (
    <li className="space-y-1 border-t border-border-button pt-2 first:border-t-0 first:pt-0">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <Badge variant="outline" className="h-5 px-1.5 text-text-secondary">
          {assertion.kind}
        </Badge>
        <span className="text-text-primary">
          {ontologyAssertionTitle(assertion)}
        </span>
        {assertion.review_status === OntologyAssertionUnreviewed && (
          <span className="text-text-disabled">
            {t('chat.ontologyAssertionUnreviewed')}
          </span>
        )}
        {/* An assertion whose sources are all outside the readable scope
            contributes vocabulary but nothing citable, and that has to be
            visible on the row rather than only in the counts around it. */}
        {chunkCount === 0 && (
          <span className="text-text-disabled">
            {t('chat.ontologyAssertionUncitable')}
          </span>
        )}
      </div>
      {assertion.evidence && (
        <blockquote className="border-l-2 border-border-button pl-2 text-xs text-text-secondary">
          {assertion.evidence}
        </blockquote>
      )}
      {attributes.length > 0 && (
        <p className="text-xs text-text-disabled">
          {attributes.map(([key, value]) => `${key}=${value}`).join(' · ')}
        </p>
      )}
    </li>
  );
}
