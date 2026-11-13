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

import { TooltipProvider } from '@/components/ui/tooltip';
import { fireEvent, render, screen } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { CompilationTemplateGroupEditor } from '.';

const mockUseForm = jest.fn();
const mockSubmit = jest.fn();
const mockSuccess = jest.fn();
const mockCancel = jest.fn();

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('./hooks/use-edit-next-compilation-template-group', () => ({
  useEditNextCompilationTemplateGroup: () => ({
    form: mockUseForm(),
    kindOptions: [],
    builtins: [],
    onSubmit: mockSubmit,
    isLoading: false,
  }),
}));

jest.mock('./components/blueprint-section', () => ({
  BlueprintSection: () => null,
}));

jest.mock('./components/template-configuration', () => ({
  TemplateConfiguration: () => {
    const { register } = jest.requireActual('react-hook-form').useFormContext();

    return (
      <input aria-label="Template name" {...register('templates.0.name')} />
    );
  },
}));

jest.mock('@/components/json-edit', () => ({
  __esModule: true,
  default: ({ value }: { value: Record<string, unknown> }) => (
    <pre data-testid="json-editor">{JSON.stringify(value)}</pre>
  ),
}));

function renderEditor({
  sidebarCollapsed,
  onSidebarCollapse,
}: {
  sidebarCollapsed?: boolean;
  onSidebarCollapse?: () => void;
} = {}) {
  return render(
    <TooltipProvider>
      <CompilationTemplateGroupEditor
        onSuccess={mockSuccess}
        onCancel={mockCancel}
        cancelText="common.back"
        sidebarCollapsed={sidebarCollapsed}
        onSidebarCollapse={onSidebarCollapse}
      />
    </TooltipProvider>,
  );
}

describe('CompilationTemplateGroupEditor', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUseForm.mockImplementation(useForm);
  });

  it('switches between configuration and live JSON without losing form values', () => {
    renderEditor({ onSidebarCollapse: jest.fn() });

    const nameInput = screen.getByRole('textbox', { name: 'Template name' });
    expect(nameInput).toHaveValue('');
    expect(screen.getByRole('heading', { name: 'common.name' })).toBeVisible();

    fireEvent.change(nameInput, { target: { value: 'Updated name' } });
    fireEvent.click(screen.getByRole('button', { name: 'JSON' }));

    expect(screen.getByTestId('json-editor')).toHaveTextContent('Updated name');
    expect(
      screen.queryByRole('textbox', { name: 'Template name' }),
    ).not.toBeInTheDocument();

    fireEvent.click(
      screen.getByRole('button', {
        name: 'knowledgeCompilation.configuration',
      }),
    );

    expect(screen.getByRole('textbox', { name: 'Template name' })).toHaveValue(
      'Updated name',
    );
    expect(screen.getByRole('heading', { name: 'Updated name' })).toBeVisible();
  });

  it('shows the page title and forwards secondary sidebar collapse', () => {
    const onSidebarCollapse = jest.fn();
    renderEditor({ onSidebarCollapse });

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.collapseSidebar' }),
    );

    expect(onSidebarCollapse).toHaveBeenCalledTimes(1);
  });

  it('hides the secondary sidebar but keeps the active content when collapsed', () => {
    const onSidebarCollapse = jest.fn();
    renderEditor({ sidebarCollapsed: true, onSidebarCollapse });

    expect(
      screen.queryByRole('button', {
        name: 'knowledgeCompilation.configuration',
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole('textbox', { name: 'Template name' }),
    ).toBeInTheDocument();
  });

  it('keeps the sidebar visible without a title or collapse button in dialogs', () => {
    renderEditor();

    expect(
      screen.getByRole('button', {
        name: 'knowledgeCompilation.configuration',
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: 'sidebar.collapseSidebar' }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('heading', { name: 'common.name' }),
    ).not.toBeInTheDocument();
  });
});
