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

import { type ComponentProps } from 'react';
import ForceGraph2D from 'react-force-graph-2d';
import { MinNodeRadius } from './node-style';
import { type ArtifactGraphNode } from './types';

export const renderNodeLabel: NonNullable<
  ComponentProps<typeof ForceGraph2D>['nodeCanvasObject']
> = (node, ctx) => {
  const graphNode = node as ArtifactGraphNode;
  const label = graphNode.name;
  const radius = graphNode.__radius ?? MinNodeRadius;
  const fontSize = 2;
  ctx.font = `${fontSize}px Sans-Serif`;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';

  const textSecondary = getComputedStyle(document.documentElement)
    .getPropertyValue('--text-secondary')
    .trim();
  ctx.fillStyle = `rgb(${textSecondary})`;

  if (typeof graphNode.x === 'number' && typeof graphNode.y === 'number') {
    ctx.fillText(label, graphNode.x, graphNode.y + radius - 9);
  }
};
