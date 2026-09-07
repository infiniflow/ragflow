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

import { forceX, forceY } from 'd3-force';
import { useEffect, type RefObject } from 'react';
import { type ForceGraphMethods } from 'react-force-graph-2d';
import { type ArtifactGraphNode } from './types';

// Weak gravity pulling every node toward the center so disconnected
// components and isolated nodes are not flung far away by charge repulsion.
const CenterGravityStrength = 0.08;

// Register weak center gravity once the graph is mounted; the forces live
// on the d3 simulation and persist across graphData changes.
export function useCenterGravity(
  fgRef: RefObject<ForceGraphMethods<ArtifactGraphNode> | undefined>,
  hasDimensions: boolean,
) {
  useEffect(() => {
    const fg = fgRef.current;
    if (!hasDimensions || !fg) return;
    fg.d3Force(
      'x',
      forceX<ArtifactGraphNode>(0).strength(CenterGravityStrength),
    );
    fg.d3Force(
      'y',
      forceY<ArtifactGraphNode>(0).strength(CenterGravityStrength),
    );
  }, [fgRef, hasDimensions]);
}
