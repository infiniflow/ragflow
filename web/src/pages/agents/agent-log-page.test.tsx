import { fireEvent, render, screen } from '@testing-library/react';
import AgentLogPage from './agent-log-page';

(globalThis as any).React = require('react');
const mockRefetch = jest.fn();

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
  useFetchAgentLog: () => ({
    data: { sessions: [], total: 0 },
    loading: false,
    refetch: mockRefetch,
  }),
}));

jest.mock('./hooks/use-export-agent-log', () => ({
  useExportAgentLogToCSV: () => ({
    handleExport: jest.fn(),
    loading: false,
  }),
}));

jest.mock('@/components/page-header', () => ({
  PageHeader: ({ children }: any) => <div>{children}</div>,
}));

jest.mock('@/components/ui/breadcrumb', () => ({
  Breadcrumb: ({ children }: any) => <div>{children}</div>,
  BreadcrumbItem: ({ children }: any) => <div>{children}</div>,
  BreadcrumbLink: ({ children }: any) => <span>{children}</span>,
  BreadcrumbList: ({ children }: any) => <div>{children}</div>,
  BreadcrumbPage: ({ children }: any) => <span>{children}</span>,
  BreadcrumbSeparator: () => null,
}));

jest.mock('@/components/ui/button', () => ({
  Button: ({ children, loading: _loading, ...props }: any) => (
    <button {...props}>{children}</button>
  ),
}));

jest.mock('@/components/ui/input', () => ({
  SearchInput: (props: any) => <input {...props} />,
}));

jest.mock('@/components/ui/ragflow-pagination', () => ({
  RAGFlowPagination: () => null,
}));

jest.mock('@/components/ui/range-picker', () => ({
  DatePickerWithRange: () => null,
}));

jest.mock('@/components/ui/spin', () => ({
  Spin: ({ children }: any) => <div>{children}</div>,
}));

jest.mock('../../components/ui/table', () => ({
  Table: ({ children }: any) => <table>{children}</table>,
  TableBody: ({ children }: any) => <tbody>{children}</tbody>,
  TableCell: ({ children, ...props }: any) => <td {...props}>{children}</td>,
  TableHead: ({ children, ...props }: any) => <th {...props}>{children}</th>,
  TableHeader: ({ children }: any) => <thead>{children}</thead>,
  TableRow: ({ children, ...props }: any) => <tr {...props}>{children}</tr>,
}));

jest.mock('./agent-log-detail-modal', () => ({
  AgentLogDetailModal: () => null,
}));

describe('AgentLogPage', () => {
  beforeEach(() => {
    mockRefetch.mockClear();
  });

  it('refetches when reset is clicked with the default filters', () => {
    render(<AgentLogPage />);

    fireEvent.click(screen.getByRole('button', { name: 'common.reset' }));

    expect(mockRefetch).toHaveBeenCalledTimes(1);
  });
});
