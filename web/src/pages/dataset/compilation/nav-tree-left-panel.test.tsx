import { fireEvent, render, screen } from '@testing-library/react';

import { GenerateStatus } from '@/constants/knowledge';

import { NavTreeLeftPanel } from './nav-tree-left-panel';

jest.mock('@/components/structure-graph/adapters', () => ({
  adaptPageIndexToTreeData: jest.fn(() => []),
  adaptTreeToTreeData: jest.fn(() => []),
  getEntityDisplayName: jest.fn((entity) => entity.name ?? ''),
}));

jest.mock('@/components/ui/tree-view', () => ({
  TreeView: ({ data }: { data: { name: string }[] }) => (
    <div data-testid="nav-tree">{data.map((node) => node.name).join(',')}</div>
  ),
}));

jest.mock('@/components/confirm-delete-dialog', () => ({
  ConfirmDeleteDialog: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
}));

jest.mock('@/components/ui/input', () => ({
  SearchInput: () => <input data-testid="nav-search" />,
}));

jest.mock('@/components/ui/spin', () => ({
  Spin: () => <div data-testid="nav-spin" />,
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/utils/backend-variant', () => ({
  useIsGoBackend: () => true,
}));

const mockCompileStatus = { value: GenerateStatus.Start };

jest.mock('@/hooks/use-dataset-generate', () => ({
  useGenerateStatus: () => ({ status: mockCompileStatus.value }),
}));

jest.mock('./update-log-sheet', () => ({
  UpdateLogSheet: ({ open, title }: { open: boolean; title: string }) =>
    open ? <div data-testid="nav-log-sheet">{title}</div> : null,
}));

const NavItem = {
  name: 'cluster-a',
  description: '',
  doc_count: 1,
  type: 'cluster',
  has_children: true,
};

const PanelHandlers = {
  onKeywordsChange: jest.fn(),
  onNodeClick: jest.fn(),
  onNodeExpand: jest.fn(),
  onEntityClick: jest.fn(),
  onDeleteAll: jest.fn(),
  onDeleteNode: jest.fn(),
};

describe('NavTreeLeftPanel', () => {
  beforeEach(() => {
    mockCompileStatus.value = GenerateStatus.Start;
  });

  it('keeps cached nodes visible when a later nav load fails', () => {
    render(
      <NavTreeLeftPanel
        navList={{ total: 1, items: [NavItem] }}
        navLoading={false}
        navError
        keywords=""
        activeKeywords=""
        childrenMap={{}}
        structureMap={{}}
        deleteNavLoading={false}
        deleteNodeLoading={false}
        {...PanelHandlers}
      />,
    );

    expect(
      screen.getByText('knowledgeCompilation.navLoadFailed'),
    ).toBeInTheDocument();
    expect(screen.getByTestId('nav-tree')).toHaveTextContent('cluster-a');
  });

  it('shows only the error placeholder when the tree is empty', () => {
    render(
      <NavTreeLeftPanel
        navList={{ total: 0, items: [] }}
        navLoading={false}
        navError
        keywords=""
        activeKeywords=""
        childrenMap={{}}
        structureMap={{}}
        deleteNavLoading={false}
        deleteNodeLoading={false}
        {...PanelHandlers}
      />,
    );

    expect(
      screen.getByText('knowledgeCompilation.navLoadFailed'),
    ).toBeInTheDocument();
    expect(screen.queryByTestId('nav-tree')).not.toBeInTheDocument();
  });

  it('shows no compile log entry when no Go compile is running', () => {
    render(
      <NavTreeLeftPanel
        navList={{ total: 1, items: [NavItem] }}
        navLoading={false}
        keywords=""
        activeKeywords=""
        childrenMap={{}}
        structureMap={{}}
        deleteNavLoading={false}
        deleteNodeLoading={false}
        {...PanelHandlers}
      />,
    );

    expect(
      screen.queryByTestId('nav-compile-log-trigger'),
    ).not.toBeInTheDocument();
  });

  it('opens the log sheet from the compile log entry while a Go compile runs', () => {
    mockCompileStatus.value = GenerateStatus.Running;
    render(
      <NavTreeLeftPanel
        navList={{ total: 1, items: [NavItem] }}
        navLoading={false}
        keywords=""
        activeKeywords=""
        childrenMap={{}}
        structureMap={{}}
        deleteNavLoading={false}
        deleteNodeLoading={false}
        traceData={{ compilationState: 'running' } as any}
        {...PanelHandlers}
      />,
    );

    fireEvent.click(screen.getByTestId('nav-compile-log-trigger'));

    expect(screen.getByTestId('nav-log-sheet')).toHaveTextContent(
      'knowledgeCompilation.navLogTitle',
    );
  });

  it('shows the error diagnostic on the log entry when a Go compile failed', () => {
    mockCompileStatus.value = GenerateStatus.Failed;
    render(
      <NavTreeLeftPanel
        navList={{ total: 1, items: [NavItem] }}
        navLoading={false}
        keywords=""
        activeKeywords=""
        childrenMap={{}}
        structureMap={{}}
        deleteNavLoading={false}
        deleteNodeLoading={false}
        traceData={{ compilationError: 'embed boom' } as any}
        {...PanelHandlers}
      />,
    );

    expect(screen.getByTestId('nav-compile-log-trigger')).toHaveTextContent(
      'embed boom',
    );
  });
});
