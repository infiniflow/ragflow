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

import { IOntologyRetrievalResult } from '@/interfaces/database/ontology-retrieval';
import { IOntologyRetrievalRequestBody } from '@/interfaces/request/ontology-retrieval';
import { retrieveOntologyAssertions } from '@/services/ontology-service';
import { useMutation } from '@tanstack/react-query';

export const enum OntologyApiAction {
  Retrieval = 'ontologyRetrieval',
}

/**
 * A knowledge-layer lookup, normalised.
 *
 * A refusal is not a transport failure: the server explains why the scope could
 * not be read (an unauthorized dataset, a model that does not exist), and that
 * sentence is what a reader needs to fix the configuration, so it survives as
 * the outcome's message instead of only reaching a toast.
 */
export interface IOntologyPreviewOutcome {
  ok: boolean;
  result?: IOntologyRetrievalResult;
  message?: string;
}

/**
 * Runs the knowledge layer for one question without saving anything.
 *
 * It is a mutation rather than a query because it is an action: the same
 * question asked twice is two searches, and a cache keyed by the question would
 * be a promise that compiled rows do not change — they do, on every publish.
 *
 * The preview is the only place the evidence grade is visible before the
 * assistant is configured with it: whether a model narrows anything, and
 * whether its quotations resolve, are facts about the corpus that no form
 * validation can establish.
 */
export const useOntologyPreview = () => {
  const {
    mutateAsync,
    isPending: loading,
    data,
    reset,
  } = useMutation({
    mutationKey: [OntologyApiAction.Retrieval],
    mutationFn: async (
      body: IOntologyRetrievalRequestBody,
    ): Promise<IOntologyPreviewOutcome> => {
      const { data } = await retrieveOntologyAssertions(body);
      if (data?.code === 0) {
        return { ok: true, result: data.data as IOntologyRetrievalResult };
      }
      return { ok: false, message: data?.message };
    },
  });

  return {
    loading,
    outcome: data,
    runPreview: mutateAsync,
    clearPreview: reset,
  };
};
