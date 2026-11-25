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
import { act, render, waitFor } from '@testing-library/react';
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

it('accepts a PAT without a username in Data Center mode and requires a username in Cloud mode', async () => {
  const formRef = createRef<DynamicFormRef>();
  const username = 'config.credentials.confluence_username';
  const onSubmit = jest.fn();
  const config = {
    is_cloud: false,
    index_mode: 'everything',
    credentials: {
      confluence_username: '',
      confluence_access_token: 'placeholder',
    },
  };
  render(
    <DynamicForm.Root
      ref={formRef}
      fields={confluenceConstant(Translate)}
      defaultValues={{ config }}
      onSubmit={onSubmit}
    />,
    { wrapper: TooltipProvider },
  );

  await act(async () => {
    expect(await formRef.current?.trigger()).toBe(true);
  });
  act(() => formRef.current?.submit());
  await waitFor(() => {
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ config: expect.objectContaining(config) }),
    );
  });

  const cloudConfig = { ...config, is_cloud: true };
  act(() => formRef.current?.reset({ config: cloudConfig }));
  await act(async () => {
    expect(await formRef.current?.trigger(username)).toBe(false);
  });

  act(() =>
    formRef.current?.reset({
      config: {
        ...cloudConfig,
        credentials: {
          ...config.credentials,
          confluence_username: 'wiki-user@example.com',
        },
      },
    }),
  );
  await act(async () => {
    expect(await formRef.current?.trigger()).toBe(true);
  });
});
