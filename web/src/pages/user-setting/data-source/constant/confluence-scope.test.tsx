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

import { DynamicForm, type DynamicFormRef } from '@/components/dynamic-form';
import { TooltipProvider } from '@/components/ui/tooltip';
import { act, fireEvent, render, screen } from '@testing-library/react';
import type { TFunction } from 'i18next';
import { createRef } from 'react';
import { confluenceConstant } from './confluence-constant';

beforeAll(() => {
  globalThis.ResizeObserver ??= class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
});

const Translate = ((key: string) => key) as TFunction;

it('renders one set of scope controls after switching modes and validating', async () => {
  const formRef = createRef<DynamicFormRef>();
  render(
    <DynamicForm.Root
      ref={formRef}
      fields={confluenceConstant(Translate)}
      defaultValues={{
        config: { index_mode: 'everything', page_id: '', space: '' },
      }}
      onSubmit={jest.fn()}
    />,
    { wrapper: TooltipProvider },
  );

  const pageId = 'setting.dataSourceFieldPageId';
  const spaceKey = 'setting.dataSourceFieldSpaceKey';
  const recursive = 'setting.dataSourceFieldIndexRecursively';

  for (let pass = 0; pass < 2; pass++) {
    fireEvent.click(screen.getByText('setting.dataSourceOptionPage'));
    await act(async () => {
      await formRef.current?.trigger([
        'config.page_id',
        'config.index_recursively',
      ]);
    });

    expect(screen.getAllByText(pageId)).toHaveLength(1);
    expect(screen.getAllByText(recursive)).toHaveLength(1);
    expect(screen.queryByText(spaceKey)).not.toBeInTheDocument();

    fireEvent.click(screen.getByText('setting.dataSourceOptionSpace'));
    expect(screen.getAllByText(spaceKey)).toHaveLength(1);
    expect(screen.queryByText(pageId)).not.toBeInTheDocument();
    expect(screen.queryByText(recursive)).not.toBeInTheDocument();

    fireEvent.click(screen.getByText('setting.dataSourceOptionEverything'));
    expect(screen.queryByText(spaceKey)).not.toBeInTheDocument();
  }
});
