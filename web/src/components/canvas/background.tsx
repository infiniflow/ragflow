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

import { Background, BackgroundVariant } from '@xyflow/react';
import { CSSProperties } from 'react';
import { CanvasBackgroundSetting } from './canvas-background';

export function AgentBackground({
  setting,
}: {
  setting?: CanvasBackgroundSetting | null;
}) {
  const solid =
    setting?.mode === 'color' && setting.color
      ? setting.color
      : 'rgb(var(--bg-canvas))';
  const image = setting?.mode === 'image' ? setting.image : undefined;
  const imageStyle: CSSProperties | undefined = image
    ? {
        backgroundImage: `url("${image}")`,
        backgroundPosition: 'center',
        backgroundRepeat: 'no-repeat',
        backgroundSize: 'cover',
      }
    : undefined;

  return (
    <Background
      id="canvas-background"
      variant={BackgroundVariant.Dots}
      color={image ? 'transparent' : 'var(--text-primary)'}
      bgColor={image ? setting?.color || 'rgb(var(--bg-canvas))' : solid}
      style={imageStyle}
      className="rounded-lg"
    />
  );
}
