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

import { useEffect } from 'react';
import useGraphStore from '../store';

/**
 * Consumes node-focus requests posted through the store (e.g. by the canvas
 * checklist in the page header, which lives outside the canvas context):
 * selects + centers the node, and opens its form sheet unless the caller only
 * wants the highlight (orphan-step issues). The request is always cleared
 * afterwards so the same node can be focused twice in a row.
 */
export function useNodeFocusRequest({
  reactFlowInstance,
  showFormDrawerById,
}: {
  reactFlowInstance?: {
    fitView: (options: {
      nodes: { id: string }[];
      duration: number;
      maxZoom: number;
    }) => void;
  };
  showFormDrawerById: (nodeId: string, toolId?: string) => void;
}) {
  const nodeFocusRequest = useGraphStore((state) => state.nodeFocusRequest);
  const selectNodeIds = useGraphStore((state) => state.selectNodeIds);
  const getNode = useGraphStore((state) => state.getNode);
  const clearNodeFocusRequest = useGraphStore(
    (state) => state.clearNodeFocusRequest,
  );

  useEffect(() => {
    if (!nodeFocusRequest) {
      return;
    }
    const { nodeId, toolId, openForm } = nodeFocusRequest;
    if (getNode(nodeId)) {
      selectNodeIds([nodeId]);
      if (openForm) {
        showFormDrawerById(nodeId, toolId);
      }
      reactFlowInstance?.fitView({
        nodes: [{ id: nodeId }],
        duration: 300,
        maxZoom: 1,
      });
    }
    clearNodeFocusRequest();
  }, [
    nodeFocusRequest,
    getNode,
    selectNodeIds,
    showFormDrawerById,
    reactFlowInstance,
    clearNodeFocusRequest,
  ]);
}
