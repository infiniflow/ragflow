import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes as RouterRoutes } from 'react-router';
import BuiltinPipelinePage from './index';

const mockSetPrimarySidebarCollapsed = jest.fn();

function MockAgentCanvas({ readOnly }: { readOnly: boolean }) {
  return <div data-testid="readonly-canvas" data-readonly={readOnly} />;
}

function MockDropdownProvider({ children }: { children: React.ReactNode }) {
  return children;
}

function MockTruncatedText({ children }: { children: React.ReactNode }) {
  return children;
}

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchBuiltinPipelines: () => ({
    data: { canvas: [{ id: 'general', title: 'General' }] },
  }),
}));

jest.mock('@/layouts/components/app-sidebar/use-sidebar-collapse', () => ({
  useSidebarCollapse: () => ({
    setCollapsed: mockSetPrimarySidebarCollapsed,
  }),
}));

jest.mock('@/components/truncated-text', () => ({
  TruncatedText: MockTruncatedText,
}));

jest.mock('../agent/canvas', () => MockAgentCanvas);
jest.mock('../agent/canvas/context', () => ({
  DropdownProvider: MockDropdownProvider,
}));

jest.mock('./use-fetch-builtin-pipeline-data', () => ({
  useFetchBuiltinPipelineDataOnMount: () => ({ loading: false }),
}));

function renderBuiltinPipeline() {
  return render(
    <MemoryRouter
      initialEntries={['/pipeline/general?category=dataflow_canvas']}
    >
      <RouterRoutes>
        <Route path="/pipeline/:id" element={<BuiltinPipelinePage />} />
      </RouterRoutes>
    </MemoryRouter>,
  );
}

describe('BuiltinPipelinePage', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders the title and Built in badge above the canvas without a secondary sidebar', () => {
    renderBuiltinPipeline();

    const header = screen.getByText('General').closest('header');
    expect(header).not.toBeNull();
    expect(header).toHaveTextContent('knowledgeConfiguration.builtInBadge');
    expect(screen.queryByRole('complementary')).not.toBeInTheDocument();
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument();
    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    expect(screen.getByTestId('readonly-canvas')).toHaveAttribute(
      'data-readonly',
      'true',
    );
  });

  it('collapses only the primary sidebar from the canvas header', () => {
    renderBuiltinPipeline();

    fireEvent.click(
      screen.getByRole('button', { name: 'sidebar.collapseSidebar' }),
    );

    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledWith(true);
    expect(mockSetPrimarySidebarCollapsed).toHaveBeenCalledTimes(1);
  });
});
