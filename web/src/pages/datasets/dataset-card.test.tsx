import { fireEvent, render, screen } from '@testing-library/react';
import { DatasetCard } from './dataset-card';

const mockSetPrimarySidebarCollapsed = jest.fn();
const mockNavigateToDataset = jest.fn();

function MockHomeCard({ onClick }: { onClick: () => void }) {
  return (
    <button data-testid="dataset-card" onClick={onClick}>
      Dataset card
    </button>
  );
}

function MockDatasetDropdown({ children }: { children: React.ReactNode }) {
  return children;
}

jest.mock('@/components/home-card', () => ({
  HomeCard: MockHomeCard,
}));
jest.mock('@/components/more-button', () => ({
  MoreButton: () => null,
}));
jest.mock('@/components/shared-badge', () => ({
  SharedBadge: () => null,
}));
jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({
    navigateToDataset: (id: string) => () => mockNavigateToDataset(id),
  }),
}));
jest.mock('@/layouts/components/app-sidebar/use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    setCollapsed: mockSetPrimarySidebarCollapsed,
  }),
}));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('./dataset-dropdown', () => ({
  DatasetDropdown: MockDatasetDropdown,
}));

describe('DatasetCard navigation', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('collapses the primary sidebar before navigating to dataset detail', () => {
    render(
      <DatasetCard
        dataset={{ id: 'dataset-1' } as never}
        showDatasetRenameModal={jest.fn()}
        showDatasetDuplicateModal={jest.fn()}
        showPrivilegeModal={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId('dataset-card'));

    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
    expect(mockNavigateToDataset).toHaveBeenCalledWith('dataset-1');
    expect(
      mockSetPrimarySidebarCollapsed.mock.invocationCallOrder[0],
    ).toBeLessThan(mockNavigateToDataset.mock.invocationCallOrder[0]);
  });
});
