import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import {
  SecondarySidebar,
  SecondarySidebarMenu,
  type SecondarySidebarMenuItem,
} from './secondary-sidebar';
import { TooltipProvider } from './ui/tooltip';

function renderSidebar({
  title,
  onCollapse,
}: {
  title?: string;
  onCollapse?: () => void;
} = {}) {
  const onRouteSelect = jest.fn();
  const onViewSelect = jest.fn();
  const items: SecondarySidebarMenuItem[] = [
    {
      key: 'route',
      icon: <span data-testid="route-icon" />,
      label: 'Route item',
      active: true,
      to: '/resource/route',
      onSelect: onRouteSelect,
    },
    {
      key: 'view',
      icon: <span data-testid="view-icon" />,
      label: 'View item',
      active: false,
      onSelect: onViewSelect,
    },
  ];

  render(
    <MemoryRouter>
      <TooltipProvider>
        <SecondarySidebar title={title} onCollapse={onCollapse}>
          <SecondarySidebarMenu items={items} navigationLabel="Resource menu" />
        </SecondarySidebar>
      </TooltipProvider>
    </MemoryRouter>,
  );

  return { onRouteSelect, onViewSelect };
}

describe('SecondarySidebar', () => {
  it('renders route and local-view items with the correct active semantics', () => {
    const { onRouteSelect, onViewSelect } = renderSidebar({
      title: 'Resource name',
    });

    const route = screen.getByRole('link', { name: 'Route item' });
    const view = screen.getByRole('button', { name: 'View item' });

    expect(screen.getByRole('heading', { name: 'Resource name' })).toHaveClass(
      'truncate',
    );
    expect(route).toHaveAttribute('aria-current', 'page');
    expect(view).toHaveAttribute('aria-pressed', 'false');

    fireEvent.click(route);
    fireEvent.click(view);

    expect(onRouteSelect).toHaveBeenCalledTimes(1);
    expect(onViewSelect).toHaveBeenCalledTimes(1);
  });

  it('forwards collapse independently from menu selection', () => {
    const onCollapse = jest.fn();
    const { onRouteSelect } = renderSidebar({
      title: 'Resource name',
      onCollapse,
    });

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.collapseSidebar' }),
    );

    expect(onCollapse).toHaveBeenCalledTimes(1);
    expect(onRouteSelect).not.toHaveBeenCalled();
  });

  it('supports dialog menus without a title or collapse control', () => {
    renderSidebar();

    expect(screen.queryByRole('heading')).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: 'sidebar.collapseSidebar' }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole('navigation', { name: 'Resource menu' }),
    ).toBeInTheDocument();
  });
});
