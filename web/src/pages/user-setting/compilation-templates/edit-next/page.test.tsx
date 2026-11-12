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

import { act, render } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import EditNextCompilationTemplate from '.';

const mockEditor = jest.fn<null, [props: unknown]>(() => null);
const mockSetSecondarySidebarCollapsed = jest.fn();
const mockSetPrimarySidebarCollapsed = jest.fn();
let mockSecondarySidebarCollapsed = false;

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/components/compilation-template-group', () => ({
  CompilationTemplateGroupEditor: (props: unknown) => mockEditor(props),
}));

jest.mock('@/hooks/use-compilation-template-sidebar-collapse', () => ({
  useCompilationTemplateSidebarCollapse: () => ({
    collapsed: mockSecondarySidebarCollapsed,
    setCollapsed: mockSetSecondarySidebarCollapsed,
  }),
}));

jest.mock('@/layouts/components/app-sidebar/use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    setCollapsed: mockSetPrimarySidebarCollapsed,
  }),
}));

type EditorProps = {
  id?: string;
  sidebarCollapsed: boolean;
  onSidebarCollapse: () => void;
};

function lastEditorProps() {
  return mockEditor.mock.calls.at(-1)?.[0] as EditorProps;
}

function renderPage() {
  return render(
    <MemoryRouter
      initialEntries={[
        '/compilation-templates/edit-next/group-1?source=agents',
      ]}
    >
      <Routes>
        <Route
          path="/compilation-templates/edit-next/:id"
          element={<EditNextCompilationTemplate />}
        />
      </Routes>
    </MemoryRouter>,
  );
}

describe('EditNextCompilationTemplate', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSecondarySidebarCollapsed = false;
  });

  it('collapses the primary sidebar and passes the route id to the editor', () => {
    renderPage();

    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
    expect(lastEditorProps().id).toBe('group-1');
    expect(lastEditorProps().sidebarCollapsed).toBe(false);
  });

  it('collapses both sidebars from the secondary sidebar control', () => {
    renderPage();

    act(() => lastEditorProps().onSidebarCollapse());

    expect(mockSetSecondarySidebarCollapsed).toHaveBeenCalledWith(true);
    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
  });

  it('expands the secondary sidebar on entry', () => {
    mockSecondarySidebarCollapsed = true;
    renderPage();

    expect(mockSetSecondarySidebarCollapsed).toHaveBeenCalledWith(false);
  });
});
