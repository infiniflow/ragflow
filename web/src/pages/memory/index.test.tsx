import { MemorySidebarCollapseProvider } from '@/hooks/use-memory-sidebar-collapse';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import MemoryLayout from './index';
import {
  SidebarCollapseProvider,
  useSidebarCollapse,
} from '@/layouts/components/app-sidebar/use-sidebar-collapse';

jest.mock('./sidebar', () => ({
  MemorySidebar: ({
    onCollapse,
    onMenuSelect,
  }: {
    onCollapse: () => void;
    onMenuSelect: () => void;
  }) => (
    <div data-testid="memory-secondary-sidebar">
      <button onClick={onMenuSelect}>Select memory section</button>
      <button onClick={onCollapse}>Collapse memory sidebar</button>
    </div>
  ),
}));

jest.mock('@/components/spotlight', () => ({
  __esModule: true,
  default: () => null,
}));

function PrimarySidebarState() {
  const { collapsed } = useSidebarCollapse();
  return (
    <span data-testid="primary-sidebar-collapsed">{String(collapsed)}</span>
  );
}

function renderLayout() {
  return render(
    <MemoryRouter>
      <SidebarCollapseProvider>
        <MemorySidebarCollapseProvider>
          <PrimarySidebarState />
          <MemoryLayout />
        </MemorySidebarCollapseProvider>
      </SidebarCollapseProvider>
    </MemoryRouter>,
  );
}

describe('MemoryLayout', () => {
  it('keeps the secondary sidebar at the top and pads only the content', () => {
    renderLayout();

    const sidebar = screen.getByTestId('memory-secondary-sidebar');

    expect(sidebar.parentElement?.parentElement).not.toHaveClass('pt-2.5');
    expect(sidebar.nextElementSibling).toHaveClass('pt-2.5');
  });

  it('collapses both sidebars from the secondary header', () => {
    renderLayout();

    fireEvent.click(
      screen.getByRole('button', { name: 'Collapse memory sidebar' }),
    );

    expect(
      screen.queryByTestId('memory-secondary-sidebar'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('primary-sidebar-collapsed')).toHaveTextContent(
      'true',
    );
  });

  it('collapses only the primary sidebar after menu navigation', () => {
    renderLayout();

    fireEvent.click(
      screen.getByRole('button', { name: 'Select memory section' }),
    );

    expect(screen.getByTestId('memory-secondary-sidebar')).toBeInTheDocument();
    expect(screen.getByTestId('primary-sidebar-collapsed')).toHaveTextContent(
      'true',
    );
  });
});
