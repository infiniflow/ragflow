import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

import DatasetWrapper from './index';

const mockSetDatasetSidebarCollapsed = jest.fn();
const mockSetPrimarySidebarCollapsed = jest.fn();
let mockDatasetSidebarCollapsed = false;

jest.mock('@/hooks/use-knowledge-request', () => ({
  useFetchKnowledgeBaseConfiguration: () => ({
    data: { name: 'Dataset', tenant_id: 'tenant-1' },
    loading: false,
  }),
}));

jest.mock('@/hooks/use-dataset-sidebar-collapse', () => ({
  useDatasetSidebarCollapse: () => ({
    collapsed: mockDatasetSidebarCollapsed,
    setCollapsed: mockSetDatasetSidebarCollapsed,
  }),
}));

jest.mock('@/layouts/components/app-sidebar/use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    setCollapsed: mockSetPrimarySidebarCollapsed,
  }),
}));

jest.mock('./sidebar', () => ({
  SideBar: ({ onMenuSelect }: { onMenuSelect: () => void }) => (
    <aside data-testid="dataset-sidebar">
      <button
        type="button"
        data-testid="dataset-menu-item"
        onClick={onMenuSelect}
      >
        Files
      </button>
    </aside>
  ),
}));

function renderDatasetRoute(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <DatasetWrapper />
    </MemoryRouter>,
  );
}

describe('DatasetWrapper', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockDatasetSidebarCollapsed = false;
  });

  it('shows both sidebars in their Compilation states', () => {
    renderDatasetRoute('/dataset/compilation/dataset-1');

    expect(screen.getByTestId('dataset-sidebar')).toBeInTheDocument();
    expect(mockSetDatasetSidebarCollapsed).toHaveBeenCalledWith(false);
    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
  });

  it('expands the secondary sidebar on entry without changing the primary sidebar', () => {
    renderDatasetRoute('/dataset/files/dataset-1');

    expect(mockSetDatasetSidebarCollapsed).toHaveBeenCalledWith(false);
    expect(mockSetPrimarySidebarCollapsed).not.toHaveBeenCalled();
  });

  it('collapses only the primary sidebar when a menu item is selected', () => {
    renderDatasetRoute('/dataset/files/dataset-1');
    mockSetDatasetSidebarCollapsed.mockClear();

    fireEvent.click(screen.getByTestId('dataset-menu-item'));

    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
    expect(mockSetDatasetSidebarCollapsed).not.toHaveBeenCalled();
  });
});
