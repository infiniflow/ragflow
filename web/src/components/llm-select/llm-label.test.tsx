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

import { render, screen } from '@testing-library/react';
import { MissingModelLabel } from './llm-label';

// Untyped on purpose: files with jest.mock are transformed by esbuild-jest's
// babel pass, which cannot strip type-only import usages.
const mockModels = [
  {
    model_id: 'm1',
    model_type: ['chat'],
    name: 'deepseek-v4-flash',
    provider_id: 'p1',
    provider_name: 'OpenAI',
    instance_id: 'i1',
    instance_name: 'geek',
  },
];

jest.mock('@/hooks/use-llm-request', () => ({
  useFetchAllAddedModels: () => ({ data: mockModels }),
}));

describe('MissingModelLabel', () => {
  it('shows the parsed name for a composite value', () => {
    render(<MissingModelLabel value="deepseek-r1@geek@OpenAI" />);

    expect(screen.getByText('deepseek-r1')).toBeInTheDocument();
  });

  it('resolves a plain model_id against the added-model list', () => {
    render(<MissingModelLabel value="m1" />);

    expect(screen.getByText('deepseek-v4-flash')).toBeInTheDocument();
  });

  it('falls back to the raw value when nothing resolves', () => {
    const { container } = render(<MissingModelLabel value="gone" />);

    expect(screen.getByText('gone')).toBeInTheDocument();
    expect(container.querySelector('svg')).toBeInTheDocument();
  });
});
