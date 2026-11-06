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

import type { IOntologyGraph } from '@/interfaces/database/document-structure';
import { useTranslation } from 'react-i18next';

/**
 * OntologyStats is the ontology's own numbers — how many classes, properties,
 * inheritance links and instances this scope produced.
 *
 * It belongs to the PAGE, not to the canvas: it was a canvas overlay, and an
 * overlay is wider than the column as soon as the detail column opens, so it
 * spilled under the panel and read as "the panel covers the stats". In the
 * header it is laid out instead of floated, and `flex-wrap` means it can never
 * overflow whatever width the column has.
 *
 * The numbers are the graph's, not the canvas's: the canvas bundles parallel
 * property edges and can hide undeclared vocabulary, both of which change what is
 * drawn but not what the ontology declares.
 */
export function OntologyStats({ graph }: { graph?: IOntologyGraph }) {
  const { t } = useTranslation();
  if (!graph) return null;
  const inheritance = graph.inheritance?.length ?? 0;
  const unattributed = graph.unattributed_relations ?? 0;
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-text-secondary">
      <span>
        {t('knowledgeCompilation.ontologyGraphStats', {
          classes: graph.classes?.length ?? 0,
          properties: graph.properties?.length ?? 0,
          entities: graph.total_entities ?? 0,
          relations: graph.total_relations ?? 0,
          defaultValue:
            '{{classes}} classes · {{properties}} properties · {{entities}} entities · {{relations}} relations',
        })}
      </span>
      {inheritance > 0 && (
        <span>
          {t('knowledgeCompilation.ontologyInheritanceCount', {
            count: inheritance,
            defaultValue: '{{count}} inheritance link(s)',
          })}
        </span>
      )}
      {unattributed > 0 && (
        // Said out loud because it is the one thing that makes the per-edge
        // counts fail to add up to the totals.
        <span>
          {t('knowledgeCompilation.ontologyUnattributed', {
            count: unattributed,
            defaultValue:
              '{{count}} relation(s) carry no class and are off the graph.',
          })}
        </span>
      )}
      {graph.counts_truncated && (
        <span>
          {t('knowledgeCompilation.ontologyCountsTruncated', {
            defaultValue: 'Counts stopped at the scan cap, so they are lower bounds.',
          })}
        </span>
      )}
    </div>
  );
}

export default OntologyStats;
