import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { TooltipProvider } from '@/components/ui/tooltip';
import { AgentCategory } from '@/constants/agent';
import Agents from './index';

// Fully isolate the agents list from the request/router/modal layers.
// The two list/builtin hooks are controlled per-test; every other hook the
// rendered subtree touches (AgentCard's dropdown/tag-editor, etc.) is stubbed
// to return an empty object so render never calls into real request code.
jest.mock(
  '@/hooks/use-agent-request',
  () =>
    new Proxy(
      {
        useFetchAgentListByPage: jest.fn(),
        useFetchBuiltinPipelines: jest.fn(),
      },
    {
      get: (target, prop: string) =>
        prop in target
          ? (target as Record<string, unknown>)[prop]
          : jest.fn(() => ({})),
    },
    ),
);
jest.mock('@/hooks/use-compilation-template-group-request', () => ({
  useDeleteCompilationTemplateGroup: () => ({ deleteGroup: jest.fn() }),
}));
jest.mock('@/hooks/logic-hooks', () => ({
  useGoToPreviousPageOnEmpty: jest.fn(),
}));
jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({
    navigateToAgent: jest.fn(),
    navigateToAgentTemplates: jest.fn(),
  }),
}));
// Avoid loading routes.tsx (it builds a data router that needs the `Request`
// global jsdom lacks); the route constants are only passed to mocked navigators.
jest.mock('@/routes', () => ({
  Routes: new Proxy({}, { get: () => '' }),
}));
jest.mock('@/pages/agents/hooks/use-create-agent', () => ({
  useCreateAgentOrPipeline: () => ({
    creatingVisible: false,
    hideCreatingModal: jest.fn(),
    showCreatingModal: jest.fn(),
    loading: false,
    handleCreateAgentOrPipeline: jest.fn(),
  }),
}));
jest.mock('@/pages/agents/hooks/use-select-filters', () => ({
  useSelectFilters: () => [],
}));
jest.mock('@/pages/agents/use-import-json', () => ({
  useHandleImportJsonFile: () => ({
    handleImportJson: jest.fn(),
    fileUploadVisible: false,
    onFileUploadOk: jest.fn(),
    hideFileUploadModal: jest.fn(),
  }),
}));
jest.mock('@/pages/agents/use-rename-agent', () => ({
  useRenameAgent: () => ({
    agentRenameLoading: false,
    initialAgentName: '',
    onAgentRenameOk: jest.fn(),
    agentRenameVisible: false,
    hideAgentRenameModal: jest.fn(),
    showAgentRenameModal: jest.fn(),
  }),
}));

// eslint-disable-next-line @typescript-eslint/no-var-requires
const agentRequest = jest.requireMock('@/hooks/use-agent-request');

const catalog = [
  {
    id: 'general',
    title: 'General',
    description: 'Default',
    filename: 'a.json',
  },
  { id: 'book', title: 'Book', description: 'Long doc', filename: 'b.json' },
];

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <MemoryRouter>
    <QueryClientProvider client={new QueryClient()}>
      <TooltipProvider>{children}</TooltipProvider>
    </QueryClientProvider>
  </MemoryRouter>
);

const defaultListPage = {
  data: [] as Array<Record<string, unknown>>,
  loading: false,
  pagination: { current: 1, pageSize: 10, total: 0 },
  setPagination: jest.fn(),
  searchString: '',
  setSearchString: jest.fn(),
  handleInputChange: jest.fn(),
  filterValue: undefined as Record<string, unknown> | undefined,
  setFilterValue: jest.fn(),
  handleFilterSubmit: jest.fn(),
  checkValue: jest.fn(),
  debouncedSearchString: '',
};

function setListPage(overrides: Partial<typeof defaultListPage> = {}) {
  agentRequest.useFetchAgentListByPage.mockReturnValue({
    ...defaultListPage,
    ...overrides,
  });
}

function setBuiltin(canvas = catalog, loading = false) {
  agentRequest.useFetchBuiltinPipelines.mockReturnValue({
    data: { canvas, total: canvas.length },
    loading,
  });
}

beforeEach(() => {
  setBuiltin();
  setListPage();
});

describe('Agents empty-state (built-in pipeline regression)', () => {
  it('shows the search-empty card on a zero-match search in the All view', () => {
    // The bug: builtinVisible (category eligibility) kept the empty
    // CardContainer mounted instead of the search-empty card.
    setListPage({
      filterValue: undefined,
      data: [],
      debouncedSearchString: 'zzz-no-match',
    });
    render(<Agents />, { wrapper });

    expect(screen.getByTestId('agents-search-empty')).toBeInTheDocument();
    expect(
      screen.queryByTestId('builtin-pipeline-section'),
    ).not.toBeInTheDocument();
  });

  it('keeps the built-in section when the search matches a built-in pipeline', () => {
    setListPage({
      filterValue: undefined,
      data: [],
      debouncedSearchString: 'general',
    });
    render(<Agents />, { wrapper });

    expect(screen.getByTestId('builtin-pipeline-section')).toBeInTheDocument();
    expect(screen.queryByTestId('agents-search-empty')).not.toBeInTheDocument();
  });

  it('shows the non-search empty card when there is nothing and no search', () => {
    setListPage({
      filterValue: undefined,
      data: [],
      debouncedSearchString: '',
    });
    setBuiltin([]); // empty catalog
    render(<Agents />, { wrapper });

    expect(screen.queryByTestId('agents-search-empty')).not.toBeInTheDocument();
    // The non-search empty card exposes the create actions.
    expect(screen.getByTestId('agents-empty-create')).toBeInTheDocument();
  });

  it('shows the search-empty card in the Agent view (built-ins not eligible)', () => {
    setListPage({
      filterValue: { canvasCategory: AgentCategory.AgentCanvas },
      data: [],
      debouncedSearchString: 'zzz-no-match',
    });
    render(<Agents />, { wrapper });

    expect(screen.getByTestId('agents-search-empty')).toBeInTheDocument();
    expect(
      screen.queryByTestId('builtin-pipeline-section'),
    ).not.toBeInTheDocument();
  });

  it('renders nothing while the user list is loading', () => {
    setListPage({ filterValue: undefined, data: [], loading: true });
    render(<Agents />, { wrapper });

    expect(screen.queryByTestId('agents-search-empty')).not.toBeInTheDocument();
    expect(screen.queryByTestId('agents-empty-create')).not.toBeInTheDocument();
  });

  it('renders the content branch with user items (and footer) without an empty card', () => {
    setListPage({
      filterValue: undefined,
      data: [
        {
          id: 'user-1',
          title: 'My Agent',
          canvas_category: AgentCategory.AgentCanvas,
          type: 'agent',
        },
      ],
      debouncedSearchString: '',
    });
    render(<Agents />, { wrapper });

    // No empty state is shown when there is at least one user item.
    expect(screen.queryByTestId('agents-search-empty')).not.toBeInTheDocument();
    expect(screen.queryByTestId('agents-empty-create')).not.toBeInTheDocument();
    // The user card itself is rendered.
    expect(screen.getByText('My Agent')).toBeInTheDocument();
  });
});
