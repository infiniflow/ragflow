/*
 * Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { fireEvent, render, screen } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';
import { WebhookRequestSchema } from '../request-schema';

jest.mock('@monaco-editor/react', () => ({ loader: { config: jest.fn() } }));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/components/collapse', () => ({
  Collapse: ({ children }: { children: React.ReactNode }) => children,
}));
jest.mock('@/components/originui/select-with-search', () => {
  const { forwardRef } = jest.requireActual<typeof import('react')>('react');
  return { SelectWithSearch: forwardRef(() => null) };
});
jest.mock('@/components/ragflow-form', () => {
  const { Controller } =
    jest.requireActual<typeof import('react-hook-form')>('react-hook-form');
  const { cloneElement } = require('react');
  return {
    RAGFlowFormItem: ({ name, children }: any) => (
      <Controller
        name={name}
        render={({ field }) =>
          typeof children === 'function'
            ? children(field)
            : cloneElement(children, field)
        }
      />
    ),
  };
});

/** Renders one editable field per request section using real form state. */
function RequestSchemaForm() {
  const form = useForm({
    defaultValues: {
      content_types: 'application/json',
      schema: {
        query: [{ key: '', type: 'string', required: false }],
        headers: [{ key: '', type: 'string', required: false }],
        body: [{ key: '', type: 'string', required: false }],
      },
    },
  });
  return (
    <FormProvider {...form}>
      <WebhookRequestSchema />
    </FormProvider>
  );
}

test('preserves HTTP header token characters and filters invalid characters', () => {
  render(<RequestSchemaForm />);
  const [, headerInput] = screen.getAllByRole('textbox');
  const headerName = "X-Request-ID!#$%&'*+.^_`|~";
  fireEvent.change(headerInput, { target: { value: `${headerName}: bad` } });
  expect(headerInput).toHaveValue(`${headerName}bad`);
});

test('keeps query and body key filtering unchanged', () => {
  render(<RequestSchemaForm />);
  const [queryInput, , bodyInput] = screen.getAllByRole('textbox');
  for (const input of [queryInput, bodyInput]) {
    fireEvent.change(input, { target: { value: 'field-name_1' } });
    expect(input).toHaveValue('fieldname_1');
  }
});
