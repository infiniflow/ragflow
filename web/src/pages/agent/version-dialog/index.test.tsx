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

import { fireEvent, render, screen } from '@testing-library/react';

jest.mock('react-i18next', () => {
  const actual = jest.requireActual('react-i18next');
  return {
    ...actual,
    useTranslation: () => ({ t: (key: string) => key }),
  };
});

jest.mock('react-router', () => ({
  useParams: () => ({ id: 'canvas-1' }),
}));

// The component imports nodeTypes from the canvas graph, which transitively loads
// the chat/i18n machinery. Mock it so the heavy canvas module is never imported.
jest.mock('../canvas', () => ({ nodeTypes: {} }));

// ReactFlow is heavy and irrelevant to the version-list assertions; render it as a
// null node so the right-panel canvas does not mount during the test.
jest.mock('@xyflow/react', () => ({
  ReactFlow: () => null,
  ReactFlowProvider: ({ children }: { children?: any }) => children,
  ConnectionMode: { Loose: 'loose' },
}));

const mockUpdateVersion = jest.fn();

const mockVersions = [
  {
    id: 'v-latest',
    title: 'Latest',
    created_at: '2024-01-02',
    release: true,
    alias: 'prod',
  },
  {
    id: 'v-old',
    title: 'Old Snapshot',
    created_at: '2024-01-01',
    release: false,
    alias: 'old',
  },
];

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchVersionList: () => ({ data: mockVersions, loading: false }),
  useFetchVersion: () => ({ data: undefined, loading: false, isError: false }),
  useDeleteVersion: () => ({
    deleteVersion: jest.fn(),
    loading: false,
    isError: false,
  }),
  useSaveAgent: () => ({ saveAgent: jest.fn(), loading: false }),
  useApplyVersion: () => ({ applyVersion: jest.fn(), loading: false }),
  useUpdateVersion: () => ({
    updateVersion: mockUpdateVersion,
    loading: false,
    isError: false,
  }),
}));

import { VersionDialog } from './index';

describe('VersionDialog outdated marker (D1/D2/D3, T21)', () => {
  it('marks non-latest versions with the ! outdated indicator but not the latest', () => {
    render(<VersionDialog hideModal={jest.fn()} />);

    expect(screen.getByText('Latest')).toBeInTheDocument();
    expect(screen.getByText('Old Snapshot')).toBeInTheDocument();
    // Exactly one ! marker: the non-latest list entry. The latest is selected by
    // default, so the detail panel shows no marker.
    expect(screen.getAllByText('!')).toHaveLength(1);
  });
});

describe('VersionDialog editable alias input (G2/section 4.4, T35)', () => {
  it('renders an editable alias input for every version', () => {
    render(<VersionDialog hideModal={jest.fn()} />);
    // Two mock versions => two alias inputs (design G2/section 4.4).
    expect(screen.getAllByPlaceholderText('flow.version.alias')).toHaveLength(
      2,
    );
  });
});

describe('VersionDialog alias filter (A2/F1, T19)', () => {
  it('narrows the list to aliases matching the query', () => {
    render(<VersionDialog hideModal={jest.fn()} />);

    const filter = screen.getByPlaceholderText('flow.version.filterByAlias');
    fireEvent.change(filter, { target: { value: 'prod' } });

    expect(screen.queryByText('Old Snapshot')).not.toBeInTheDocument();
    expect(screen.getByText('Latest')).toBeInTheDocument();
    expect(screen.queryByText('!')).not.toBeInTheDocument();
  });

  it('shows every version when the alias filter is cleared', () => {
    render(<VersionDialog hideModal={jest.fn()} />);

    const filter = screen.getByPlaceholderText('flow.version.filterByAlias');
    fireEvent.change(filter, { target: { value: 'prod' } });
    fireEvent.change(filter, { target: { value: '' } });

    expect(screen.getByText('Latest')).toBeInTheDocument();
    expect(screen.getByText('Old Snapshot')).toBeInTheDocument();
    expect(screen.getAllByText('!')).toHaveLength(1);
  });
});

describe('VersionDialog alias edit commits on blur (G2/section 4.4, T35)', () => {
  it('PATCHes the trimmed new alias when the input loses focus', () => {
    mockUpdateVersion.mockClear();
    render(<VersionDialog hideModal={jest.fn()} />);

    // mockVersions order: [v-latest (alias prod), v-old (alias old)].
    const inputs = screen.getAllByPlaceholderText('flow.version.alias');
    const oldInput = inputs[1];

    fireEvent.change(oldInput, { target: { value: '  renamed  ' } });
    fireEvent.blur(oldInput);

    expect(mockUpdateVersion).toHaveBeenCalledTimes(1);
    expect(mockUpdateVersion).toHaveBeenCalledWith({
      canvasId: 'canvas-1',
      versionId: 'v-old',
      alias: 'renamed',
    });
  });

  it('does not PATCH when the alias is unchanged', () => {
    mockUpdateVersion.mockClear();
    render(<VersionDialog hideModal={jest.fn()} />);

    const inputs = screen.getAllByPlaceholderText('flow.version.alias');
    const oldInput = inputs[1]; // alias old

    fireEvent.change(oldInput, { target: { value: 'old' } });
    fireEvent.blur(oldInput);

    expect(mockUpdateVersion).not.toHaveBeenCalled();
  });
});
