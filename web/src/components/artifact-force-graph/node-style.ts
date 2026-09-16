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

import { type IArtifactGraphEntity } from '@/interfaces/database/dataset';

export const EntityNodeColor = '#00BEB4';
export const ConceptNodeColor = '#4CACFF';

export const MinNodeRadius = 4;
export const MaxNodeRadius = 14;

export const DefaultLinkWidth = 1;
export const HighlightLinkWidth = 2;

export const DimmedAlpha = 0.2;

export const getNodeColor = (node: IArtifactGraphEntity): string => {
  if (node.type === 'entity') return EntityNodeColor;
  if (node.type === 'concept') return ConceptNodeColor;
  return EntityNodeColor;
};

export const getNodeRadius = (
  node: IArtifactGraphEntity,
  minWeight: number,
  maxWeight: number,
): number => {
  const weight = node.weight ?? 0;
  if (maxWeight <= minWeight) return MinNodeRadius;
  const clamped = Math.max(minWeight, Math.min(maxWeight, weight));
  const t = (clamped - minWeight) / (maxWeight - minWeight);
  return MinNodeRadius + t * (MaxNodeRadius - MinNodeRadius);
};
