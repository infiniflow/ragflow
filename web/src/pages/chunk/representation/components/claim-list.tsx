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
import type {
  IClaimEvidence,
  IClaimItem,
} from '@/interfaces/database/document-structure';
import { ChevronDown, ChevronUp, Loader2, X } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

// Verbatim quotes rendered as indented citation blocks. `expandable` shows the
// first quote and folds the rest behind a toggle so long lists stay scannable.
export function EvidenceBlock({
  items,
  expandable = true,
}: {
  items: IClaimEvidence[];
  expandable?: boolean;
}) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);

  const evidence = items.filter((e) => e.quote?.trim());
  if (!evidence.length) return null;
  const shown = expandable && !expanded ? evidence.slice(0, 1) : evidence;

  return (
    <div className="mt-2 space-y-1.5">
      {shown.map((item, idx) => (
        <blockquote
          key={`${item.chunk_id}-${idx}`}
          className="border-l-2 border-accent/40 pl-2 text-xs italic leading-5 text-text-secondary"
        >
          {item.quote}
        </blockquote>
      ))}
      {expandable && evidence.length > 1 && (
        <button
          type="button"
          className="mt-1.5 inline-flex items-center gap-1 text-xs text-accent hover:underline"
          onClick={() => setExpanded((prev) => !prev)}
        >
          {expanded ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
          {expanded
            ? t('knowledgeCompilation.claimsCollapseEvidence', {
                defaultValue: 'Show less',
              })
            : t('knowledgeCompilation.claimsMoreEvidence', {
                count: evidence.length - 1,
                defaultValue: `+${evidence.length - 1} more evidence`,
              })}
        </button>
      )}
    </div>
  );
}

function ClaimCard({ claim }: { claim: IClaimItem }) {
  return (
    <div className="rounded-lg border border-border-button bg-bg-card px-3 py-2.5">
      <div className="text-sm font-medium text-text-primary">{claim.name}</div>
      {claim.description && claim.description !== claim.name && (
        <div className="mt-1 text-xs text-text-secondary">
          {claim.description}
        </div>
      )}
      <EvidenceBlock items={claim.evidence ?? []} />
    </div>
  );
}

interface ClaimListProps {
  claims: IClaimItem[];
}

export function ClaimList({ claims }: ClaimListProps) {
  const { t } = useTranslation();

  if (!claims.length) {
    return (
      <div className="py-4 text-sm text-text-secondary">
        {t('knowledgeCompilation.claimsEmpty', {
          defaultValue: 'No claims were extracted for this cluster.',
        })}
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {claims.map((claim, idx) => (
        <ClaimCard key={`${claim.name}-${idx}`} claim={claim} />
      ))}
    </div>
  );
}

// The artifact page renders these panels as a resizable middle column, so the
// tree view only owns the selection and hands the resolved content upward. The
// payload carries its own close handler: clearing the selection lives here,
// next to the click handler that set it.
export interface ClaimsPanelState {
  clusterName?: string;
  claims: IClaimItem[];
  total: number;
  loading: boolean;
  onClose: () => void;
}

export interface EvidencePanelState {
  nodeName?: string;
  description?: string;
  evidence: IClaimEvidence[];
  onClose: () => void;
}

// One leaf cluster's claims, opened from the tree's count badge. Rendered as a
// full-height column between the artifact tree and the chunk list, so it shares
// the frame language of those panels: a bordered header on top, a scrollable
// body filling the rest, and a close affordance on the right.
export function ClaimsPanel({
  clusterName,
  claims,
  total,
  loading,
  onClose,
}: ClaimsPanelState) {
  const { t } = useTranslation();

  return (
    <article className="h-full flex flex-col">
      <header className="flex items-center justify-between gap-2 flex-0 p-5 pb-2.5 border-b-0.5 border-b-border-button">
        <h2 className="truncate text-[18px] text-text-primary">
          {t('knowledgeCompilation.claimsPanelTitle', {
            name: clusterName ?? '',
            defaultValue: `Claims · ${clusterName ?? ''}`,
          })}
        </h2>
        <div className="flex items-center gap-2 shrink-0">
          {!loading && (
            <span className="text-[14px] text-text-secondary">
              {t('knowledgeCompilation.claimsTotal', {
                count: total,
                defaultValue: `${total} total`,
              })}
            </span>
          )}
          <Button
            variant="ghost"
            size="icon"
            type="button"
            className="h-6 w-6"
            onClick={onClose}
            aria-label={t('common.close', 'Close')}
          >
            <X size={14} />
          </Button>
        </div>
      </header>
      <div className="flex-1 h-0 overflow-auto scrollbar-auto px-5 pb-5 pt-2.5">
        {loading ? (
          <div className="flex items-center justify-center gap-2 py-6 text-sm text-text-secondary">
            <Loader2 className="animate-spin" size={16} />
            {t('knowledgeCompilation.claimsLoading', {
              defaultValue: 'Loading claims…',
            })}
          </div>
        ) : (
          <ClaimList claims={claims} />
        )}
      </div>
    </article>
  );
}

interface NodeDetailPanelProps {
  nodeName?: string;
  description?: string;
  evidence: IClaimEvidence[];
  onClose: () => void;
}

// A single tree node's detail (page_index fact/conclusion): its description
// plus the gate-verified quotes backing it, shown when the user clicks the
// node in the tree. Shares the citation rendering with the claims panel.
export function NodeDetailPanel({
  nodeName,
  description,
  evidence,
  onClose,
}: NodeDetailPanelProps) {
  const { t } = useTranslation();

  return (
    <article className="h-full flex flex-col">
      <header className="flex items-center justify-between gap-2 flex-0 p-5 pb-2.5 border-b-0.5 border-b-border-button">
        <h2 className="truncate text-[18px] text-text-primary">
          {nodeName ||
            t('knowledgeCompilation.claimsNodeDetail', {
              defaultValue: 'Details',
            })}
        </h2>
        <Button
          variant="ghost"
          size="icon"
          type="button"
          className="h-6 w-6 shrink-0"
          onClick={onClose}
          aria-label={t('common.close', 'Close')}
        >
          <X size={14} />
        </Button>
      </header>
      <div className="flex-1 h-0 overflow-auto scrollbar-auto px-5 pb-5 pt-2.5">
        {description && (
          <p className="text-sm leading-6 text-text-secondary">{description}</p>
        )}
        <EvidenceBlock items={evidence} expandable />
      </div>
    </article>
  );
}
