import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

const mockSetFilesSidebarCollapsed = jest.fn();
const mockSetPrimarySidebarCollapsed = jest.fn();
let mockFilesSidebarCollapsed = false;

jest.mock('@/hooks/use-files-sidebar-collapse', () => ({
  useFilesSidebarCollapse: () => ({
    collapsed: mockFilesSidebarCollapsed,
    setCollapsed: mockSetFilesSidebarCollapsed,
  }),
}));

jest.mock('@/layouts/components/app-sidebar/use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    setCollapsed: mockSetPrimarySidebarCollapsed,
  }),
}));

jest.mock('./sidebar', () => ({
  FilesSidebar: ({
    onCollapse,
    onMenuSelect,
  }: {
    onCollapse: () => void;
    onMenuSelect: () => void;
  }) => (
    <aside data-testid="files-sidebar">
      <button type="button" onClick={onCollapse}>
        Collapse
      </button>
      <button type="button" onClick={onMenuSelect}>
        All files
      </button>
    </aside>
  ),
}));

import FilesLayout from './layout';

function renderFilesLayout() {
  return render(
    <MemoryRouter initialEntries={['/files']}>
      <FilesLayout />
    </MemoryRouter>,
  );
}

describe('FilesLayout', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockFilesSidebarCollapsed = false;
  });

  it('wraps the right-hand content in the shared PageFrame', () => {
    const { container } = renderFilesLayout();

    const pageFrame = container.querySelector('.page-frame');
    expect(pageFrame).toHaveClass('pr-2.5', 'pb-2.5');
    expect(pageFrame?.firstElementChild).toHaveClass(
      'h-full',
      'min-h-0',
      'overflow-hidden',
    );
  });

  it('collapses both sidebars from the Library header control', () => {
    renderFilesLayout();
    expect(mockSetFilesSidebarCollapsed).toHaveBeenCalledWith(false);
    mockSetFilesSidebarCollapsed.mockClear();

    fireEvent.click(screen.getByRole('button', { name: 'Collapse' }));

    expect(mockSetFilesSidebarCollapsed).toHaveBeenCalledWith(true);
    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
  });

  it('collapses only the primary sidebar when All files is selected', () => {
    renderFilesLayout();
    mockSetFilesSidebarCollapsed.mockClear();

    fireEvent.click(screen.getByRole('button', { name: 'All files' }));

    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
    expect(mockSetFilesSidebarCollapsed).not.toHaveBeenCalled();
  });

  it('hides the Library sidebar while collapsed', () => {
    mockFilesSidebarCollapsed = true;
    renderFilesLayout();

    expect(screen.queryByTestId('files-sidebar')).not.toBeInTheDocument();
  });
});
