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
import { OntologyEvidenceLevel } from '@/interfaces/database/ontology-retrieval';
import { cn } from '@/lib/utils';
import { ontologyEvidenceLabel } from '@/utils/ontology-evidence';
import { useTranslation } from 'react-i18next';

/**
 * The grade as a reader sees it, on both surfaces that show one: the preview
 * panel where a setting is checked, and the answer that was steered by it.
 */
export function OntologyEvidenceBadge({ level }: { level?: string }) {
  const { t } = useTranslation();
  const structured = level === OntologyEvidenceLevel.Structured;
  const none = level === OntologyEvidenceLevel.None;

  return (
    <Badge
      variant="outline"
      className={cn(
        'h-5 px-1.5',
        structured && 'text-state-success',
        none && 'text-text-disabled',
        !structured && !none && 'text-state-warning',
      )}
    >
      {ontologyEvidenceLabel(level, t)}
    </Badge>
  );
}
