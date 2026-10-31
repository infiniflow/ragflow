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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { OntologyDetailPanel } from './detail-panel';

// `any` on purpose throughout: this file contains jest.mock, so it goes through
// esbuild-jest's babel pass, which cannot strip imported bindings used as type
// annotations (same constraint noted in use-memory-request.test.tsx).
const mockClassEntities = jest.fn();
const mockPropertyRelations = jest.fn();

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    // Interpolates the {{placeholders}}, like the real i18n does. Without it a
    // test would assert on the raw template and pass on text no user ever sees.
    t: (key: string, options?: any) => {
      if (!options) return key;
      const template: string = options.defaultValue ?? key;
      return template.replace(/{{\s*(\w+)\s*}}/g, (_match: string, name: string) =>
        String(options[name] ?? ''),
      );
    },
  }),
}));

jest.mock('@/services/document-structure-service', () => ({
  __esModule: true,
  default: {
    getOntologyClassEntities: (...args: unknown[]) =>
      mockClassEntities(...args),
    getOntologyPropertyRelations: (...args: unknown[]) =>
      mockPropertyRelations(...args),
  },
}));

const renderPanel = (state: any) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <OntologyDetailPanel {...state} />
    </QueryClientProvider>,
  );
};

/** A class WITH instances — the case that must open a list, not a dead end. */
const classWithInstances = (overrides: any = {}): any => ({
  className: 'person',
  label: 'Person',
  entities: 52,
  declared: true,
  parents: [],
  children: [],
  attributes: [],
  properties: [],
  datasetId: 'ds-1',
  templateId: 'tpl-1',
  onClose: jest.fn(),
  ...overrides,
});

describe('OntologyDetailPanel drill-down', () => {
  beforeEach(() => {
    mockClassEntities.mockReset();
    mockPropertyRelations.mockReset();
    mockClassEntities.mockResolvedValue({
      data: {
        data: {
          class: 'person',
          total: 1,
          offset: 0,
          limit: 20,
          entities: [{ id: 'e1', name: 'Ada Lovelace' }],
        },
      },
    });
  });

  // The reported symptom was "clicking does nothing and no request is made", and
  // the first suspect is always a disabled button: it swallows the click, prints
  // nothing and fires nothing, so a reader cannot tell it apart from a bug.
  it('offers an ENABLED way in when the class has instances', () => {
    renderPanel(classWithInstances());
    const button = screen.getByText('View instances').closest('button');
    expect(button).not.toBeNull();
    expect((button as HTMLButtonElement).disabled).toBe(false);
  });

  it('opens the instance list and asks the service for the scope', async () => {
    const state = classWithInstances();
    renderPanel(state);

    fireEvent.click(screen.getByText('View instances'));

    // The list replaces the class view, so the way back is what proves we moved.
    expect(await screen.findByText('← Person')).toBeTruthy();
    await waitFor(() => {
      expect(mockClassEntities).toHaveBeenCalledWith(
        'ds-1',
        'person',
        expect.objectContaining({ template_id: 'tpl-1' }),
      );
    });
    // Drilling in is navigation INSIDE the panel — it must not close the panel.
    expect(state.onClose).not.toHaveBeenCalled();
  });

  // A disabled button with no explanation is a dead end; the class view must say
  // why there is nothing to open instead.
  it('explains itself instead of offering a dead button at zero instances', () => {
    renderPanel(classWithInstances({ entities: 0 }));
    expect(screen.queryByText('View instances')).toBeNull();
    expect(screen.getByText(/No instance of this class was compiled/)).toBeTruthy();
    // And nothing is requested, because there is nothing to ask for.
    expect(mockClassEntities).not.toHaveBeenCalled();
  });

  // "52 instances" must not mean "20 rows and a dead count": the window grows on
  // demand, and the panel says how much is still off screen.
  it('grows the window on demand instead of stopping at one page', async () => {
    mockClassEntities.mockResolvedValue({
      data: {
        data: {
          class: 'person',
          total: 52,
          offset: 0,
          limit: 20,
          entities: Array.from({ length: 20 }, (_, i) => ({
            id: `e${i}`,
            name: `person ${i}`,
          })),
        },
      },
    });
    renderPanel(classWithInstances());

    fireEvent.click(screen.getByText('View instances'));
    expect(await screen.findByText('32 more not shown')).toBeTruthy();

    fireEvent.click(screen.getByText('Load 20 more'));
    await waitFor(() => {
      expect(mockClassEntities).toHaveBeenLastCalledWith(
        'ds-1',
        'person',
        expect.objectContaining({ limit: 40 }),
      );
    });
  });
});
