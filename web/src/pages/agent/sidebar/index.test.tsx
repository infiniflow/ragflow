import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes as RouterRoutes } from 'react-router';
import { AgentSidebar } from './index';

function MockTruncatedText({ children }: { children: React.ReactNode }) {
  return children;
}

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/components/truncated-text', () => ({
  TruncatedText: MockTruncatedText,
}));

function renderSidebar(
  entry: string,
  onMenuSelect: jest.Mock = jest.fn(),
  onCollapse: jest.Mock = jest.fn(),
) {
  render(
    <MemoryRouter initialEntries={[entry]}>
      <RouterRoutes>
        <Route
          path="/agent/:id/*"
          element={
            <AgentSidebar
              title="Agent title"
              onCollapse={onCollapse}
              onMenuSelect={onMenuSelect}
            />
          }
        />
      </RouterRoutes>
    </MemoryRouter>,
  );

  return { onMenuSelect, onCollapse };
}

describe('AgentSidebar', () => {
  it('shows Canvas, Launch and Setting for a workflow', () => {
    renderSidebar('/agent/agent-1');

    expect(screen.getByRole('link', { name: 'flow.canvas' })).toHaveAttribute(
      'aria-current',
      'page',
    );
    expect(
      screen.getByRole('link', { name: 'explore.title' }),
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'flow.setting' })).toHaveAttribute(
      'href',
      '/agent/agent-1/setting',
    );
  });

  it('hides Launch and preserves the category for a pipeline', () => {
    renderSidebar('/agent/pipeline-1?category=dataflow_canvas');

    expect(
      screen.queryByRole('link', { name: 'explore.title' }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'flow.canvas' })).toHaveAttribute(
      'href',
      '/agent/pipeline-1?category=dataflow_canvas',
    );
    expect(screen.getByRole('link', { name: 'flow.setting' })).toHaveAttribute(
      'href',
      '/agent/pipeline-1/setting?category=dataflow_canvas',
    );
  });

  it('marks the current secondary page active', () => {
    renderSidebar('/agent/agent-1/setting');

    expect(
      screen.getByRole('link', { name: 'flow.canvas' }),
    ).not.toHaveAttribute('aria-current');
    expect(screen.getByRole('link', { name: 'flow.setting' })).toHaveAttribute(
      'aria-current',
      'page',
    );
  });

  it('notifies menu selection and secondary sidebar collapse separately', () => {
    const { onMenuSelect, onCollapse } = renderSidebar('/agent/agent-1');

    fireEvent.click(screen.getByRole('link', { name: 'explore.title' }));
    expect(onMenuSelect).toHaveBeenCalledTimes(1);
    expect(onCollapse).not.toHaveBeenCalled();

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.collapseSidebar' }),
    );
    expect(onCollapse).toHaveBeenCalledTimes(1);
  });
});
