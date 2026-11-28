import { fireEvent, render, screen } from '@testing-library/react';
import { LucidePlug } from 'lucide-react';
import { Link, MemoryRouter } from 'react-router';
import { CollapsibleNavGroup } from './nav-item';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('./use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({ collapsed: false }),
}));

jest.mock('@/ee/hooks/use-visible-integration-children', () => ({
  useVisibleIntegrationChildren: (items: unknown[]) => items,
}));

const Children = [
  { key: 'model', path: '/integrations/model', labelKey: 'Model' },
  { key: 'mcp', path: '/integrations/mcp', labelKey: 'MCP' },
];

function renderGroup(initialPath: string) {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <CollapsibleNavGroup
        id="integrations-nav-children"
        labelKey="Integrations"
        icon={LucidePlug}
        items={Children}
      />
      <Link to="/">Outside</Link>
      <Link to="/integrations/model">Model page</Link>
      <Link to="/integrations/mcp">MCP page</Link>
    </MemoryRouter>,
  );
}

describe('CollapsibleNavGroup', () => {
  it('expands after navigating to another child even when manually collapsed', () => {
    renderGroup('/integrations/model');

    const group = screen.getByRole('button', { name: 'Integrations' });
    expect(group).toHaveAttribute('aria-expanded', 'true');

    fireEvent.click(group);
    expect(group).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('link', { name: 'MCP' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('link', { name: 'MCP page' }));
    expect(group).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('link', { name: 'MCP' })).toHaveAttribute(
      'aria-current',
      'page',
    );
  });

  it('expands when entering a child route and preserves expansion when leaving it', () => {
    renderGroup('/');

    const group = screen.getByRole('button', { name: 'Integrations' });
    expect(group).toHaveAttribute('aria-expanded', 'false');

    fireEvent.click(screen.getByRole('link', { name: 'Model page' }));
    expect(group).toHaveAttribute('aria-expanded', 'true');

    fireEvent.click(screen.getByRole('link', { name: 'Outside' }));
    expect(group).toHaveAttribute('aria-expanded', 'true');
  });
});
