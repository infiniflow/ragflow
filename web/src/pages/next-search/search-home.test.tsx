import { render, screen } from '@testing-library/react';
import SearchHome from './search-home';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/hooks/use-auto-resize-textarea', () => ({
  useAutoResizeTextarea: () => false,
}));
jest.mock('@/components/spotlight', () => ({
  __esModule: true,
  default: () => <div data-testid="mock-spotlight" />,
}));

function renderSearchHome(showLogo: boolean) {
  return render(
    <SearchHome
      isSearching={false}
      setIsSearching={jest.fn()}
      searchText=""
      setSearchText={jest.fn()}
      showLogo={showLogo}
    />,
  );
}

describe('SearchHome branding', () => {
  it('keeps the welcome frame while hiding the internal detail logo', () => {
    renderSearchHome(false);

    expect(
      screen.queryByRole('heading', { name: 'RAGFlow' }),
    ).not.toBeInTheDocument();
    expect(screen.getByText('👋 Hi there')).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText('search.searchGreeting'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('👋 Hi there').parentElement?.parentElement,
    ).toHaveClass('border', 'min-h-[240px]');
  });

  it('keeps the logo by default for the public share page', () => {
    render(
      <SearchHome
        isSearching={false}
        setIsSearching={jest.fn()}
        searchText=""
        setSearchText={jest.fn()}
      />,
    );

    expect(
      screen.getByRole('heading', { name: 'RAGFlow' }),
    ).toBeInTheDocument();
  });
});
