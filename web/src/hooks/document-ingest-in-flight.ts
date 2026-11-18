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

const documentIngestInFlight = new Map<string, Promise<unknown>>();

export const sendDocumentIngest = (
  params: {
    documentIds: string[];
    run: number;
    option?: { delete: boolean; apply_kb: boolean };
  },
  request: () => Promise<unknown>,
  { force = false }: { force?: boolean } = {},
) => {
  const key = JSON.stringify({
    documentIds: [...params.documentIds].sort(),
    run: params.run,
    option: params.option || null,
  });
  // The stop-loss retry must reach the server even while an identical request
  // is still pending, so it bypasses the in-flight entry.
  if (!force) {
    const existingRequest = documentIngestInFlight.get(key);
    if (existingRequest) {
      return existingRequest;
    }
  }

  const inFlight = request();
  documentIngestInFlight.set(key, inFlight);
  const clearRequest = () => {
    if (documentIngestInFlight.get(key) === inFlight) {
      documentIngestInFlight.delete(key);
    }
  };
  void inFlight.then(clearRequest, clearRequest);
  return inFlight;
};
