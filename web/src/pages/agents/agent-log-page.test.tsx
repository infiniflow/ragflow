import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import AgentLogPage from './agent-log-page';

type IAgentLogsRequest =
  import('@/interfaces/database/agent').IAgentLogsRequest;
type IAgentLogsResponse =
  import('@/interfaces/database/agent').IAgentLogsResponse;
type ChangeEvent<T> = import('react').ChangeEvent<T>;

const mockFetchLogs = jest.fn();
const mockExportLogs = jest.fn();
const mockLogKeys = {
  list: (params: IAgentLogsRequest) => ['agent-logs', params] as const,
};

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchAgentLog: (params: IAgentLogsRequest) => {
    const { useQuery } = jest.requireActual('@tanstack/react-query');
    const query = useQuery({
      queryKey: mockLogKeys.list(params),
      queryFn: () => mockFetchLogs(params),
      gcTime: 0,
    });
    return {
      data: query.data,
      loading: query.isFetching,
      refetch: query.refetch,
    };
  },
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: () => ({ id: 'agent-1' }),
}));
jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({
    navigateToAgents: jest.fn(),
    navigateToAgent: () => jest.fn(),
  }),
}));
jest.mock('../agent/hooks/use-fetch-data', () => ({
  useFetchDataOnMount: () => ({ flowDetail: { title: 'Support agent' } }),
}));
jest.mock('./agent-log-detail-modal', () => ({
  AgentLogDetailModal: () => null,
}));
jest.mock('./hooks/use-export-agent-log', () => ({
  useExportAgentLogToCSV: () => ({
    handleExport: mockExportLogs,
    loading: false,
  }),
}));

// Keep the page's date/pagination state real while replacing popup widgets.
jest.mock('@/components/ui/range-picker', () => ({
  DatePickerWithRange: ({
    selected,
    onSelect,
  }: {
    selected: { from?: Date; to?: Date };
    onSelect: (range: { from?: Date; to?: Date }) => void;
  }) => {
    const handleFromChange = (event: ChangeEvent<HTMLInputElement>) =>
      onSelect({ ...selected, from: new Date(event.target.value) });
    const handleToChange = (event: ChangeEvent<HTMLInputElement>) =>
      onSelect({ ...selected, to: new Date(event.target.value) });
    const handleReselect = () =>
      onSelect({
        from: selected.from && new Date(selected.from),
        to: selected.to && new Date(selected.to),
      });
    return (
      <>
        <input
          aria-label="From date"
          value={selected.from?.toISOString() ?? ''}
          onChange={handleFromChange}
        />
        <input
          aria-label="To date"
          value={selected.to?.toISOString() ?? ''}
          onChange={handleToChange}
        />
        <button onClick={handleReselect}>Reselect date range</button>
      </>
    );
  },
}));
jest.mock('@/components/originui/select-with-search', () => ({
  SelectWithSearch: ({
    value,
    onChange,
  }: {
    value: string;
    onChange: (value: string) => void;
  }) => {
    const handleChange = (event: ChangeEvent<HTMLSelectElement>) =>
      onChange(event.target.value);
    return (
      <select aria-label="Page size" value={value} onChange={handleChange}>
        <option value="10">10</option>
        <option value="20">20</option>
      </select>
    );
  },
}));

const LogResponse: IAgentLogsResponse = {
  total: 100,
  sessions: [
    {
      id: 'session-1',
      message: [
        { id: 'message-1', role: 'user', content: 'Saved conversation' },
      ],
      update_date: '2026-09-20T12:00:00Z',
      create_date: '2026-09-20T11:00:00Z',
      update_time: 1789905600000,
      create_time: 1789902000000,
      round: 1,
      thumb_up: 0,
      errors: '',
      source: 'agent',
      user_id: 'user-1',
      dsl: '{}',
      reference: [],
      name: 'Conversation',
      version_title: 'v1',
    },
  ],
};

const DraftFrom = '2026-09-10T00:00:00.000Z';
const DraftTo = '2026-09-12T23:59:59.999Z';

function changeKeywords(value: string) {
  fireEvent.change(screen.getByPlaceholderText('common.search'), {
    target: { value },
  });
}

function changeDateRange() {
  fireEvent.change(screen.getByLabelText('From date'), {
    target: { value: DraftFrom },
  });
  fireEvent.change(screen.getByLabelText('To date'), {
    target: { value: DraftTo },
  });
}

function clickSearch() {
  fireEvent.click(screen.getByRole('button', { name: 'common.search' }));
}

async function waitForLogs() {
  await screen.findByText('Saved conversation');
  expect(screen.queryByText('Loading...')).not.toBeInTheDocument();
}

async function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <AgentLogPage />
    </QueryClientProvider>,
  );
  await waitForLogs();
}

describe('AgentLogPage applied search filters', () => {
  beforeEach(() => {
    mockFetchLogs.mockReset().mockResolvedValue(LogResponse);
    mockExportLogs.mockReset();
  });

  it('keeps keyword and date drafts out of queries until Search is clicked', async () => {
    await renderPage();
    const initialParams = mockFetchLogs.mock.calls[0][0];

    changeKeywords('c');
    changeKeywords('conversation');
    changeDateRange();

    expect(screen.getByPlaceholderText('common.search')).toHaveValue(
      'conversation',
    );
    expect(screen.getByLabelText('From date')).toHaveValue(DraftFrom);
    expect(mockFetchLogs).toHaveBeenCalledTimes(1);
    expect(mockFetchLogs).toHaveBeenLastCalledWith(initialParams);

    clickSearch();
    await waitForLogs();

    expect(mockFetchLogs).toHaveBeenCalledTimes(2);
    expect(mockFetchLogs).toHaveBeenLastCalledWith({
      keywords: 'conversation',
      from_date: new Date(DraftFrom),
      to_date: new Date(DraftTo),
      orderby: 'create_time',
      desc: false,
      page: 1,
      page_size: 10,
    });
  });

  it('searches from page one while preserving the chosen page size and sort', async () => {
    await renderPage();
    fireEvent.change(screen.getByRole('combobox', { name: 'Page size' }), {
      target: { value: '20' },
    });
    await waitForLogs();
    fireEvent.click(
      screen.getByRole('columnheader', { name: 'flow.latestDate' }),
    );
    await waitForLogs();
    fireEvent.click(screen.getByRole('link', { name: '3' }));
    await waitForLogs();
    changeKeywords('latest draft');
    changeDateRange();

    clickSearch();
    await waitForLogs();

    expect(mockFetchLogs).toHaveBeenLastCalledWith({
      keywords: 'latest draft',
      from_date: new Date(DraftFrom),
      to_date: new Date(DraftTo),
      orderby: 'update_date',
      desc: false,
      page: 1,
      page_size: 20,
    });
    expect(screen.getByRole('combobox', { name: 'Page size' })).toHaveValue(
      '20',
    );
    expect(
      screen.getByRole('columnheader', { name: /flow.latestDate/ }),
    ).toHaveTextContent('↑');

    fireEvent.click(screen.getByRole('link', { name: 'Go to next page' }));
    await waitForLogs();
    expect(mockFetchLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({
        keywords: 'latest draft',
        page: 2,
        page_size: 20,
      }),
    );
  });

  it('uses applied filters when paging, sorting and exporting with pending edits', async () => {
    await renderPage();
    changeKeywords('applied');
    changeDateRange();
    clickSearch();
    await waitForLogs();
    changeKeywords('pending');
    fireEvent.change(screen.getByLabelText('From date'), {
      target: { value: '2026-09-11T00:00:00.000Z' },
    });

    fireEvent.click(await screen.findByRole('link', { name: '2' }));
    await waitForLogs();
    expect(mockFetchLogs).toHaveBeenLastCalledWith({
      keywords: 'applied',
      from_date: new Date(DraftFrom),
      to_date: new Date(DraftTo),
      orderby: 'create_time',
      desc: false,
      page: 2,
      page_size: 10,
    });

    fireEvent.click(
      screen.getByRole('columnheader', { name: 'flow.latestDate' }),
    );
    await waitForLogs();
    fireEvent.click(
      screen.getByRole('columnheader', { name: /flow.latestDate/ }),
    );
    await waitForLogs();
    expect(mockFetchLogs).toHaveBeenLastCalledWith({
      keywords: 'applied',
      from_date: new Date(DraftFrom),
      to_date: new Date(DraftTo),
      orderby: 'update_date',
      desc: true,
      page: 2,
      page_size: 10,
    });
    expect(screen.getByPlaceholderText('common.search')).toHaveValue('pending');

    fireEvent.change(screen.getByRole('combobox', { name: 'Page size' }), {
      target: { value: '20' },
    });
    await waitForLogs();
    fireEvent.click(
      screen.getByRole('button', { name: 'flow.exportCurrentPage' }),
    );
    const expectedParams = {
      keywords: 'applied',
      from_date: new Date(DraftFrom),
      to_date: new Date(DraftTo),
      orderby: 'update_date',
      desc: true,
      page: 1,
      page_size: 20,
    };
    expect(mockFetchLogs).toHaveBeenLastCalledWith(expectedParams);
    expect(mockExportLogs).toHaveBeenCalledWith(expectedParams);
  });

  it.each([false, true])(
    'refetches unchanged filters on explicit Search (reselected dates: %s)',
    async (reselectDates) => {
      await renderPage();
      if (reselectDates) {
        fireEvent.click(
          screen.getByRole('button', { name: 'Reselect date range' }),
        );
      }

      clickSearch();
      await waitFor(() => expect(mockFetchLogs).toHaveBeenCalledTimes(2));
      await waitForLogs();
      expect(mockFetchLogs.mock.calls[1][0]).toEqual(
        mockFetchLogs.mock.calls[0][0],
      );
    },
  );

  it('resets drafts, applied filters, sorting and page while retaining page size', async () => {
    await renderPage();
    const initialParams = mockFetchLogs.mock.calls[0][0];
    fireEvent.change(screen.getByRole('combobox', { name: 'Page size' }), {
      target: { value: '20' },
    });
    await waitForLogs();
    changeKeywords('applied');
    changeDateRange();
    clickSearch();
    await waitForLogs();
    fireEvent.click(
      screen.getByRole('columnheader', { name: 'flow.latestDate' }),
    );
    await waitForLogs();
    fireEvent.click(screen.getByRole('link', { name: '3' }));
    await waitForLogs();
    changeKeywords('pending');

    fireEvent.click(screen.getByRole('button', { name: 'common.reset' }));
    await waitForLogs();

    expect(mockFetchLogs).toHaveBeenLastCalledWith({
      ...initialParams,
      keywords: '',
      orderby: 'create_time',
      desc: false,
      page: 1,
      page_size: 20,
    });
    expect(screen.getByPlaceholderText('common.search')).toHaveValue('');
    expect(screen.getByLabelText('From date')).toHaveValue(
      initialParams.from_date.toISOString(),
    );
    expect(screen.getByLabelText('To date')).toHaveValue(
      initialParams.to_date.toISOString(),
    );
    expect(
      screen.getByRole('columnheader', { name: 'flow.latestDate' }),
    ).not.toHaveTextContent(/[↑↓]/);

    const requestCount = mockFetchLogs.mock.calls.length;
    fireEvent.click(screen.getByRole('button', { name: 'common.reset' }));
    expect(mockFetchLogs).toHaveBeenCalledTimes(requestCount);

    fireEvent.click(screen.getByRole('link', { name: 'Go to next page' }));
    await waitForLogs();
    expect(mockFetchLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({
        keywords: '',
        page: 2,
        page_size: 20,
        orderby: 'create_time',
      }),
    );
  });
});
