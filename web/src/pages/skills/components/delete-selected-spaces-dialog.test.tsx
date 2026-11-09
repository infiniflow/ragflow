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
import { createInstance } from 'i18next';
import { I18nextProvider, initReactI18next } from 'react-i18next';
import { DeleteSelectedSpacesDialog } from './delete-selected-spaces-dialog';

jest.mock('@/components/ui/dialog', () => ({
  Dialog: ({ children }: { children: JSX.Element }) => <div>{children}</div>,
  DialogContent: ({ children }: { children: JSX.Element }) => (
    <div>{children}</div>
  ),
  DialogDescription: ({ children }: { children: JSX.Element }) => (
    <p>{children}</p>
  ),
  DialogFooter: ({ children }: { children: JSX.Element }) => (
    <footer>{children}</footer>
  ),
  DialogHeader: ({ children }: { children: JSX.Element }) => (
    <header>{children}</header>
  ),
  DialogTitle: ({ children }: { children: JSX.Element }) => <h2>{children}</h2>,
}));

describe('DeleteSelectedSpacesDialog translations', () => {
  it('uses the English defaults when the locale has no matching keys', async () => {
    const i18n = createInstance();
    await i18n.use(initReactI18next).init({
      lng: 'en',
      fallbackLng: false,
      resources: { en: { translation: {} } },
    });

    render(
      <I18nextProvider i18n={i18n}>
        <DeleteSelectedSpacesDialog
          open
          onOpenChange={jest.fn()}
          selectedCount={3}
          onDelete={jest.fn()}
        />
      </I18nextProvider>,
    );

    expect(
      screen.getByRole('heading', { name: 'Delete Selected Spaces' }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        'Are you sure you want to delete 3 selected spaces? This action cannot be undone.',
      ),
    ).toBeInTheDocument();
  });
});
