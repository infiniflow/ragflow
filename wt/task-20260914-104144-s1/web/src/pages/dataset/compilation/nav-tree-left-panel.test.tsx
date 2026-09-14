import { render, screen } from '@testing-library/react';

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
  it('keeps cached nodes visible when a later nav load fails', () => {
    render(
      <NavTreeLeftPanel
        navList={{ total: 1, items: [NavItem] }}
        navLoading={false}
        navError
        keywords=""
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
});
