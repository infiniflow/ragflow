import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { NavGroupListFilterBar } from './components/nav-group-list-filter-bar';
import NavGroupLayout from './nav-group-layout';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

function renderListPage(pathname: string) {
  return render(
    <MemoryRouter initialEntries={[pathname]}>
      <Routes>
        <Route element={<NavGroupLayout />}>
          <Route path="/datasets" element={<NavGroupListFilterBar />} />
          <Route path="/memories" element={<NavGroupListFilterBar />} />
          <Route path="/chats" element={<NavGroupListFilterBar />} />
          <Route path="/searches" element={<NavGroupListFilterBar />} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

describe('NavGroupLayout list page header', () => {
  it.each([
    ['/datasets', 'header.dataset', 'sidebar.context'],
    ['/memories', 'header.memories', 'sidebar.context'],
    ['/chats', 'header.chat', 'sidebar.application'],
    ['/searches', 'header.search', 'sidebar.application'],
  ])(
    'renders %s with the title above tabs and the toolbar on the same row',
    (pathname, tabLabel, title) => {
      renderListPage(pathname);

      const heading = screen.getByRole('heading', { name: title });
      const tab = screen.getByText(tabLabel);
      const toolbar = screen.getByRole('toolbar');
      const tabPanel = tab.closest('h1');

      expect(heading.compareDocumentPosition(tab)).toBe(
        Node.DOCUMENT_POSITION_FOLLOWING,
      );
      expect(tab.compareDocumentPosition(toolbar)).toBe(
        Node.DOCUMENT_POSITION_FOLLOWING,
      );
      expect(tabPanel?.parentElement).toBe(toolbar.parentElement);
      expect(toolbar.parentElement).toHaveClass(
        'md:flex-row',
        'md:justify-between',
      );
      expect(heading.querySelector('svg')).not.toBeInTheDocument();
      expect(tab.querySelector('svg')).not.toBeInTheDocument();
    },
  );
});
