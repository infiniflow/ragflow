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

import api from '@/utils/api';
import request from '@/utils/next-request';

export const getDocumentStructureGraph = (
  datasetId: string,
  documentId: string,
  keywords?: string,
) =>
  request.get(api.documentStructureGraph(datasetId, documentId), {
    params: keywords ? { keywords } : undefined,
  });

export const getDocumentStructureClaims = (
  datasetId: string,
  documentId: string,
  params: {
    chunk_ids?: string;
    template_id?: string;
    offset?: number;
    limit?: number;
  } = {},
) =>
  request.get(api.documentStructureClaims(datasetId, documentId), {
    params,
  });

export const deleteDocumentStructureGraph = (
  datasetId: string,
  documentId: string,
  templateId: string,
) =>
  request.delete(api.documentStructureGraph(datasetId, documentId), {
    data: { template_id: templateId },
  });

/**
 * One page of a class's instances — the drill-down behind a class node of the
 * ontology model graph. `template_id` / `document_id` narrow the scope; leaving
 * both out asks the whole dataset.
 */
export const getOntologyClassEntities = (
  datasetId: string,
  classType: string,
  params: {
    template_id?: string;
    document_id?: string;
    offset?: number;
    limit?: number;
  } = {},
) =>
  request.get(api.ontologyClassEntities(datasetId, classType), {
    params,
  });

/**
 * One page of a property edge's assertions. `source_type` / `target_type` pin one
 * domain -> range combination of a polymorphic property, which is the edge the
 * graph drew.
 */
export const getOntologyPropertyRelations = (
  datasetId: string,
  property: string,
  params: {
    source_type?: string;
    target_type?: string;
    template_id?: string;
    document_id?: string;
    offset?: number;
    limit?: number;
  } = {},
) =>
  request.get(api.ontologyPropertyRelations(datasetId, property), {
    params,
  });

const documentStructureService = {
  getDocumentStructureGraph,
  getDocumentStructureClaims,
  deleteDocumentStructureGraph,
  getOntologyClassEntities,
  getOntologyPropertyRelations,
};

export default documentStructureService;
