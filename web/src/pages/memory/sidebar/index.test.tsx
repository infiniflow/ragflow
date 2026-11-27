import { TooltipProvider } from '@/components/ui/tooltip';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes as RouterRoutes } from 'react-router';
import { MemorySidebar } from './index';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('../hooks/use-memory-setting', () => ({
  useFetchMemoryBaseConfiguration: () => ({
    data: {
      name: 'Memory title',
      avatar: 'avatar.png',
      description: 'Private description',
      create_time: 1764144000,
    },
  }),
}));

function renderSidebar(
  onMenuSelect: jest.Mock = jest.fn(),
  onCollapse: jest.Mock = jest.fn(),
) {
  render(
    <MemoryRouter initialEntries={['/memory/memory-setting/memory-1']}>
      <TooltipProvider>
        <RouterRoutes>
          <Route
            path="/memory/:section/:id"
            element={
              <MemorySidebar
                onCollapse={onCollapse}
                onMenuSelect={onMenuSelect}
              />
            }
          />
        </RouterRoutes>
      </TooltipProvider>
    </MemoryRouter>,
  );

  return { onMenuSelect, onCollapse };
}

describe('MemorySidebar', () => {
  it('shows only the title and memory-specific navigation', () => {
    renderSidebar();

    expect(
      screen.getByRole('heading', { name: 'Memory title' }),
    ).toBeInTheDocument();
    expect(screen.queryByText('Private description')).not.toBeInTheDocument();
    expect(
      screen.queryByText(/knowledgeDetails.created/),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole('img')).not.toBeInTheDocument();

    expect(
      screen.getByRole('link', { name: 'memory.sideBar.messages' }),
    ).toHaveAttribute('href', '/memory/memory-message/memory-1');
    expect(
      screen.getByRole('link', { name: 'memory.sideBar.configuration' }),
    ).toHaveAttribute('href', '/memory/memory-setting/memory-1');
  });

  it('marks the active route and forwards menu and collapse actions', () => {
    const { onMenuSelect, onCollapse } = renderSidebar();

    const messages = screen.getByRole('link', {
      name: 'memory.sideBar.messages',
    });
    const settings = screen.getByRole('link', {
      name: 'memory.sideBar.configuration',
    });

    expect(messages).not.toHaveAttribute('aria-current');
    expect(settings).toHaveAttribute('aria-current', 'page');

    fireEvent.click(messages);
    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.collapseSidebar' }),
    );

    expect(onMenuSelect).toHaveBeenCalledTimes(1);
    expect(onCollapse).toHaveBeenCalledTimes(1);
  });
});
