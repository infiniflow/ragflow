import { render, screen, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { TooltipProvider } from '@/components/ui/tooltip';
import { BuiltinPipelineSection } from './builtin-pipeline-section';
import { AgentCategory } from '@/constants/agent';

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchBuiltinPipelines: jest.fn(),
}));

const mockCopy = jest.fn();
jest.mock('./use-copy-builtin-pipeline', () => ({
  useCopyBuiltinPipeline: () => ({ copy: mockCopy, copying: false }),
}));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const agentRequest = require('@/hooks/use-agent-request');

const catalog = [
  {
    id: 'general',
    title: 'General',
    description: 'Default',
    filename: 'a.json',
  },
  {
    id: 'book',
    title: 'Book',
    description: 'Long doc',
    filename: 'b.json',
  },
];

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <QueryClientProvider client={new QueryClient()}>
    <TooltipProvider>{children}</TooltipProvider>
  </QueryClientProvider>
);

beforeEach(() => {
  mockCopy.mockReset();
  agentRequest.useFetchBuiltinPipelines.mockReturnValue({
    data: { canvas: catalog, total: catalog.length },
    loading: false,
  });
});

describe('BuiltinPipelineSection', () => {
  it('renders the section with cards in the "All" view', () => {
    const { container } = render(
      <BuiltinPipelineSection rawCategory={undefined} searchString="" />,
      { wrapper },
    );
    expect(screen.getByTestId('builtin-pipeline-section')).toBeInTheDocument();
    expect(
      screen.getByText('knowledgeConfiguration.builtInPipelines'),
    ).toBeInTheDocument();
    // Two built-in cards rendered (each carries a copy button).
    expect(screen.getAllByTestId('copy-builtin-pipeline')).toHaveLength(2);
    // The cards are laid out in the same responsive grid as the user items
    // (not a single long column).
    const grid = container.querySelector('.grid');
    expect(grid).not.toBeNull();
    expect(
      grid?.querySelectorAll('[data-testid="copy-builtin-pipeline"]'),
    ).toHaveLength(2);
  });

  it('still renders when the user list is empty (built-ins are not gated on user items)', () => {
    // This is the empty-user-list visibility scenario: the section must appear
    // in the All view even though there are zero user-owned canvases.
    const { container } = render(
      <BuiltinPipelineSection rawCategory={undefined} searchString="general" />,
      { wrapper },
    );
    expect(screen.getByTestId('builtin-pipeline-section')).toBeInTheDocument();
    expect(screen.getAllByTestId('copy-builtin-pipeline')).toHaveLength(1);
    expect(
      container.querySelector('[data-testid="builtin-badge"]'),
    ).toBeTruthy();
  });

  it('hides the section in the pure Agent view', () => {
    render(
      <BuiltinPipelineSection
        rawCategory={AgentCategory.AgentCanvas}
        searchString=""
      />,
      { wrapper },
    );
    expect(
      screen.queryByTestId('builtin-pipeline-section'),
    ).not.toBeInTheDocument();
  });

  it('hides the section for a structured (non-string) category filter', () => {
    render(
      <BuiltinPipelineSection
        rawCategory={
          { operator: 'or', values: [AgentCategory.DataflowCanvas] } as any
        }
        searchString=""
      />,
      { wrapper },
    );
    expect(
      screen.queryByTestId('builtin-pipeline-section'),
    ).not.toBeInTheDocument();
  });

  it('hides the section when the catalog is empty', () => {
    agentRequest.useFetchBuiltinPipelines.mockReturnValue({
      data: { canvas: [], total: 0 },
      loading: false,
    });
    render(<BuiltinPipelineSection rawCategory={undefined} searchString="" />, {
      wrapper,
    });
    expect(
      screen.queryByTestId('builtin-pipeline-section'),
    ).not.toBeInTheDocument();
  });

  it('renders a divider before the section only when showDivider is set', () => {
    const { rerender } = render(
      <BuiltinPipelineSection
        rawCategory={undefined}
        searchString=""
        showDivider={false}
      />,
      { wrapper },
    );
    expect(document.querySelector('.border-t')).toBeNull();

    rerender(
      <BuiltinPipelineSection
        rawCategory={undefined}
        searchString=""
        showDivider
      />,
    );
    expect(document.querySelector('.border-t')).not.toBeNull();
  });

  it('triggers the copy mutation when a card copy button is clicked', () => {
    render(<BuiltinPipelineSection rawCategory={undefined} searchString="" />, {
      wrapper,
    });
    fireEvent.click(screen.getAllByTestId('copy-builtin-pipeline')[0]);
    expect(mockCopy).toHaveBeenCalledTimes(1);
    expect(mockCopy.mock.calls[0][0]).toEqual({
      id: 'general',
      title: 'General',
    });
  });
});
