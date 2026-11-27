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

import documentStructureService from '@/services/document-structure-service';
import { keepPreviousData, useQuery } from '@tanstack/react-query';

/** A panel shows a page, not a dump. */
export const AboxPageSize = 30;

/**
 * The canvas node budget.
 *
 * Not a page size: a list can say "12–30 of 240" and still reach row 240, while
 * a canvas has to decide up front how much it draws. 60 keeps a bubble graph
 * readable; the response reports the remainder so the UI can point at the list.
 */
export const AboxNodeBudget = 60;

interface GraphArgs {
  datasetId: string;
  documentId?: string;
  templateId?: string;
  class?: string;
  hop: number;
  enabled: boolean;
}

/** The bounded subgraph behind the instance canvas. */
export function useOntologyGraph({
  datasetId,
  documentId,
  templateId,
  class: entityClass,
  hop,
  enabled,
}: GraphArgs) {
  return useQuery({
    queryKey: [
      'ontology-abox-graph',
      datasetId,
      documentId,
      templateId,
      entityClass,
      hop,
    ],
    enabled: enabled && !!datasetId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      documentStructureService.getOntologyGraph(datasetId, {
        class: entityClass || undefined,
        template_id: templateId,
        document_id: documentId,
        hop,
        limit: AboxNodeBudget,
      }),
  });
}

interface RowArgs {
  datasetId: string;
  documentId?: string;
  templateId?: string;
  kind: 'entity' | 'relation';
  class?: string;
  property?: string;
  sourceType?: string;
  targetType?: string;
  offset: number;
  enabled: boolean;
}

/** One page of rows — the only view that can reach the last one. */
export function useOntologyRows({
  datasetId,
  documentId,
  templateId,
  kind,
  class: entityClass,
  property,
  sourceType,
  targetType,
  offset,
  enabled,
}: RowArgs) {
  return useQuery({
    queryKey: [
      'ontology-abox-rows',
      datasetId,
      documentId,
      templateId,
      kind,
      entityClass,
      property,
      sourceType,
      targetType,
      offset,
    ],
    enabled: enabled && !!datasetId,
    placeholderData: keepPreviousData,
    queryFn: () =>
      documentStructureService.getOntologyArtifacts(datasetId, {
        kind,
        class: entityClass || undefined,
        property: property || undefined,
        source_type: sourceType || undefined,
        target_type: targetType || undefined,
        template_id: templateId,
        document_id: documentId,
        offset,
        limit: AboxPageSize,
      }),
  });
}
