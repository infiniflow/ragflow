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

import { resolveAgentsUrl } from './utils';

describe('resolveAgentsUrl', () => {
  it('restores the Compilation tab for the agents source', () => {
    expect(resolveAgentsUrl('agents')).toBe(
      '/agents?canvas_category=compilation_template_group',
    );
  });

  it('keeps the plain agents route for other entry points', () => {
    expect(resolveAgentsUrl(null)).toBe('/agents');
    expect(resolveAgentsUrl('settings')).toBe('/agents');
  });
});
