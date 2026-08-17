import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import AgentLogPage from './agent-log-page';

const mockRefetch = jest.fn();
const mockFetchAgentLog = jest.fn();

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('react-router', () => ({
  useParams: () => ({ id: 'agent-1' }),
}));

jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({
    navigateToAgents: jest.fn(),
    navigateToAgent: () => jest.fn(),
  }),
}));

jest.mock('../agent/hooks/use-fetch-data', () => ({
  useFetchDataOnMount: () => ({
    flowDetail: { title: 'Agent' },
  }),
}));

jest.mock('@/hooks/use-agent-request', () => ({
  useFetchAgentLog: (searchParams: unknown) => {
    mockFetchAgentLog(searchParams);
    return {
      data: { sessions: [], total: 0 },
      loading: false,
      refetch: mockRefetch,
    };
  },
}));

jest.mock('./hooks/use-export-agent-log', () => ({
  useExportAgentLogToCSV: () => ({
    handleExport: jest.fn(),
    loading: false,
  }),
}));

jest.mock('@/components/page-header', () => ({
  PageHeader: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

jest.mock('@/components/ui/breadcrumb', () => ({
  Breadcrumb: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  BreadcrumbItem: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  BreadcrumbLink: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
  BreadcrumbList: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  BreadcrumbPage: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
  BreadcrumbSeparator: () => null,
}));

jest.mock('@/components/ui/button', () => ({
  Button: ({
    children,
    loading: _loading,
    ...props
  }: React.ButtonHTMLAttributes<HTMLButtonElement> & { loading?: boolean }) => (
    <button {...props}>{children}</button>
  ),
}));

jest.mock('@/components/ui/input', () => ({
  SearchInput: (props: React.InputHTMLAttributes<HTMLInputElement>) => (
    <input {...props} />
  ),
}));

jest.mock('@/components/ui/ragflow-pagination', () => ({
  RAGFlowPagination: ({
    current,
    pageSize,
    onChange,
  }: {
    current: number;
    pageSize: number;
    onChange: (page: number, pageSize: number) => void;
  }) => (
    <div>
      <span data-testid="pagination-state">{`${current}:${pageSize}`}</span>
      <button type="button" onClick={() => onChange(1, 25)}>
        change page size
      </button>
      <button type="button" onClick={() => onChange(3, 25)}>
        change page
      </button>
    </div>
  ),
}));

jest.mock('@/components/ui/range-picker', () => ({
  DatePickerWithRange: () => null,
}));

jest.mock('@/components/ui/spin', () => ({
  Spin: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

jest.mock('../../components/ui/table', () => ({
  Table: ({ children }: { children: React.ReactNode }) => (
    <table>{children}</table>
  ),
  TableBody: ({ children }: { children: React.ReactNode }) => (
    <tbody>{children}</tbody>
  ),
  TableCell: ({
    children,
    ...props
  }: React.TdHTMLAttributes<HTMLTableCellElement>) => (
    <td {...props}>{children}</td>
  ),
  TableHead: ({
    children,
    ...props
  }: React.ThHTMLAttributes<HTMLTableCellElement>) => (
    <th {...props}>{children}</th>
  ),
  TableHeader: ({ children }: { children: React.ReactNode }) => (
    <thead>{children}</thead>
  ),
  TableRow: ({
    children,
    ...props
  }: React.HTMLAttributes<HTMLTableRowElement>) => (
    <tr {...props}>{children}</tr>
  ),
}));

jest.mock('./agent-log-detail-modal', () => ({
  AgentLogDetailModal: () => null,
}));

describe('AgentLogPage reset', () => {
  beforeEach(() => {
    mockRefetch.mockClear();
    mockFetchAgentLog.mockClear();
  });

  it('refetches logs when Reset is clicked with the default query already active', () => {
    render(<AgentLogPage />);

    fireEvent.click(screen.getByRole('button', { name: 'common.reset' }));

    expect(mockRefetch).toHaveBeenCalledTimes(1);
  });

  it('returns to page 1 on Reset without dropping the current page size', async () => {
    render(<AgentLogPage />);

    fireEvent.click(screen.getByRole('button', { name: 'change page size' }));
    fireEvent.click(screen.getByRole('button', { name: 'change page' }));

    expect(screen.getByTestId('pagination-state').textContent).toBe('3:25');

    mockFetchAgentLog.mockClear();
    mockRefetch.mockClear();
    fireEvent.click(screen.getByRole('button', { name: 'common.reset' }));

    await waitFor(() => {
      expect(screen.getByTestId('pagination-state').textContent).toBe('1:25');
      expect(mockFetchAgentLog).toHaveBeenLastCalledWith(
        expect.objectContaining({
          page: 1,
          page_size: 25,
          orderby: 'create_time',
          desc: false,
          keywords: '',
        }),
      );
    });
    expect(mockRefetch).not.toHaveBeenCalled();
  });
});
