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
import { AgentCategory } from '@/constants/agent';
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';

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

const mockApplyVersion = jest.fn();
const mockNavigateToAgent = jest.fn();
const mockNavigate = jest.fn();
const mockUpdateVersion = jest.fn();
let mockIsPipeline = false;

jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({ navigateToAgent: mockNavigateToAgent }),
}));

jest.mock('../hooks/use-is-pipeline', () => ({
  useIsPipeline: () => mockIsPipeline,
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

// One released version and one unpublished legacy snapshot: the page lists
// every row and marks only the released one with a Dot.
// `let` so pagination tests can swap in a larger list before rendering.
let mockVersions = [
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
  useApplyVersion: () => ({
    applyVersion: mockApplyVersion,
    loading: false,
  }),
  useUpdateVersion: () => ({
    updateVersion: mockUpdateVersion,
    loading: false,
    isError: false,
  }),
}));

import AgentVersionHistory from './index';

// TooltipProvider lives at the app root in production; tests must supply it.
const renderPage = () =>
  render(
    <TooltipProvider>
      <AgentVersionHistory />
    </TooltipProvider>,
  );

const getRow = (title: string) => screen.getByText(title).closest('li')!;

async function restoreLatestVersion() {
  const restoreButton = screen.getByRole('button', {
    name: 'flow.version.restore',
  });
  await waitFor(() => expect(restoreButton).toBeEnabled());
  fireEvent.click(restoreButton);

  const confirmation = screen.getByRole('alertdialog');
  await act(async () => {
    fireEvent.click(
      within(confirmation).getByRole('button', {
        name: 'flow.version.restore',
      }),
    );
  });
}

beforeEach(() => {
  jest.clearAllMocks();
  mockIsPipeline = false;
  mockNavigateToAgent.mockReturnValue(mockNavigate);
});

describe('AgentVersionHistory release marker', () => {
  it('lists unpublished legacy versions alongside released ones', () => {
    renderPage();

    expect(screen.getByText('Latest')).toBeInTheDocument();
    expect(screen.getByText('Old Snapshot')).toBeInTheDocument();
  });

  it('marks only released rows with the accent dot', () => {
    renderPage();

    expect(
      getRow('Latest').querySelector('.bg-accent-primary'),
    ).toBeInTheDocument();
    expect(
      getRow('Old Snapshot').querySelector('.bg-accent-primary'),
    ).not.toBeInTheDocument();
  });
});

describe('AgentVersionHistory alias edit entry (G2/section 4.4, T35)', () => {
  it('renders an edit-alias button for every version', () => {
    renderPage();
    // Two mock versions => one edit-alias button per row.
    expect(within(getRow('Latest')).getByRole('button')).toBeInTheDocument();
    expect(
      within(getRow('Old Snapshot')).getByRole('button'),
    ).toBeInTheDocument();
  });

  it('opens the alias dialog with the current alias when the button is clicked', () => {
    renderPage();

    fireEvent.click(within(getRow('Old Snapshot')).getByRole('button'));

    expect(screen.getByPlaceholderText('flow.version.alias')).toHaveValue(
      'old',
    );
  });

  it('PATCHes the trimmed new alias on confirm', async () => {
    renderPage();

    fireEvent.click(within(getRow('Old Snapshot')).getByRole('button'));
    fireEvent.change(screen.getByPlaceholderText('flow.version.alias'), {
      target: { value: '  renamed  ' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'common.confirm' }));

    await waitFor(() => {
      expect(mockUpdateVersion).toHaveBeenCalledTimes(1);
    });
    expect(mockUpdateVersion).toHaveBeenCalledWith({
      canvasId: 'canvas-1',
      versionId: 'v-old',
      alias: 'renamed',
    });
  });

  it('does not PATCH when the dialog is cancelled', () => {
    renderPage();

    fireEvent.click(within(getRow('Old Snapshot')).getByRole('button'));
    fireEvent.click(screen.getByRole('button', { name: 'common.cancel' }));

    expect(mockUpdateVersion).not.toHaveBeenCalled();
    expect(
      screen.queryByPlaceholderText('flow.version.alias'),
    ).not.toBeInTheDocument();
  });
});

describe('AgentVersionHistory alias filter (A2/F1, T19)', () => {
  it('narrows the list to aliases matching the query', () => {
    renderPage();

    const filter = screen.getByPlaceholderText('flow.version.filterByAlias');
    fireEvent.change(filter, { target: { value: 'prod' } });

    expect(screen.queryByText('Old Snapshot')).not.toBeInTheDocument();
    expect(screen.getByText('Latest')).toBeInTheDocument();
  });

  it('shows every version when the alias filter is cleared', () => {
    renderPage();

    const filter = screen.getByPlaceholderText('flow.version.filterByAlias');
    fireEvent.change(filter, { target: { value: 'prod' } });
    fireEvent.change(filter, { target: { value: '' } });

    expect(screen.getByText('Latest')).toBeInTheDocument();
    expect(screen.getByText('Old Snapshot')).toBeInTheDocument();
  });

  it('jumps back to page 1 when filtering shrinks the list', () => {
    const originalVersions = mockVersions;
    // 12 released versions overfill one 10-row page; only the last (page-2)
    // row carries an alias, so filtering must surface it from page 1.
    mockVersions = Array.from({ length: 12 }, (_, i) => ({
      id: `v-${i}`,
      title: `Version ${i}`,
      created_at: '2024-01-01',
      release: true,
      alias: i === 11 ? 'special' : '',
    }));

    try {
      renderPage();

      fireEvent.click(screen.getByText('2'));
      expect(screen.queryByText('Version 0')).not.toBeInTheDocument();
      expect(screen.getByText('Version 11')).toBeInTheDocument();

      const filter = screen.getByPlaceholderText('flow.version.filterByAlias');
      fireEvent.change(filter, { target: { value: 'special' } });

      // Without the page reset, page 2 would slice past the single match and
      // render an empty list.
      expect(screen.getByText('Version 11')).toBeInTheDocument();
      expect(screen.queryByText('2')).not.toBeInTheDocument();
    } finally {
      mockVersions = originalVersions;
    }
  });
});

describe('AgentVersionHistory restore navigation', () => {
  it('returns a restored workflow to its agent canvas', async () => {
    mockApplyVersion.mockResolvedValue(true);
    renderPage();

    await restoreLatestVersion();

    expect(mockApplyVersion).toHaveBeenCalledWith({
      canvasId: 'canvas-1',
      versionId: 'v-latest',
    });
    expect(mockNavigateToAgent).toHaveBeenCalledWith(
      'canvas-1',
      AgentCategory.AgentCanvas,
    );
    expect(mockNavigate).toHaveBeenCalledTimes(1);
  });

  it('returns a restored pipeline to its dataflow canvas', async () => {
    mockIsPipeline = true;
    mockApplyVersion.mockResolvedValue(true);
    renderPage();

    await restoreLatestVersion();

    expect(mockNavigateToAgent).toHaveBeenCalledWith(
      'canvas-1',
      AgentCategory.DataflowCanvas,
    );
    expect(mockNavigate).toHaveBeenCalledTimes(1);
  });

  it('stays on the page when restoring fails', async () => {
    mockApplyVersion.mockResolvedValue(false);
    renderPage();

    await restoreLatestVersion();

    expect(mockApplyVersion).toHaveBeenCalledTimes(1);
    expect(mockNavigateToAgent).not.toHaveBeenCalled();
    expect(mockNavigate).not.toHaveBeenCalled();
  });
});
