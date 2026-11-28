import { render, screen } from '@testing-library/react';
import dayjs from 'dayjs';

const mockKnowledgeBase = {
  create_time: 1741964019000,
  size: 1610612736,
};

const mockDocumentCounts = {
  parsingCount: 12,
  pendingCount: 2,
};

jest.mock('@/utils/file-util', () => ({
  formatBytes: jest.fn(() => '1.5 GB'),
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: { count?: number }) =>
      options?.count === undefined ? key : `${key}:${options.count}`,
  }),
}));

jest.mock('@/hooks/use-document-request', () => ({
  useFetchDocumentList: jest.fn(),
  useGetDocumentFilter: () => mockDocumentCounts,
}));

jest.mock('../contexts/knowledge-base-context', () => ({
  useKnowledgeBaseContext: () => ({ knowledgeBase: mockKnowledgeBase }),
}));

jest.mock('./use-dataset-table-columns', () => ({
  useDatasetTableColumns: () => [],
}));

jest.mock('./use-change-document-parser', () => ({
  useChangeDocumentParser: () => ({
    changeParserLoading: false,
    onChangeParserOk: jest.fn(),
    changeParserVisible: false,
    hideChangeParserModal: jest.fn(),
    showChangeParserModal: jest.fn(),
    changeParserRecord: {},
    reparseVisible: false,
    hideReparseModal: jest.fn(),
    confirmReparse: jest.fn(),
  }),
}));

jest.mock('./use-rename-document', () => ({
  useRenameDocument: () => ({
    renameLoading: false,
    onRenameOk: jest.fn(),
    renameVisible: false,
    hideRenameModal: jest.fn(),
    showRenameModal: jest.fn(),
    initialName: '',
  }),
}));

jest.mock('./hooks', () => ({
  useShowLog: () => ({
    showLog: jest.fn(),
    logInfo: null,
    logVisible: false,
    datasetId: '',
    logId: '',
  }),
}));

jest.mock('./change-parser-dialog', () => ({
  ChangeParserDialog: () => null,
  ReparseConfirmDialog: () => null,
}));

jest.mock('@/components/rename-dialog', () => ({
  RenameDialog: () => null,
}));

jest.mock('../process-log-modal', () => ({
  __esModule: true,
  default: () => null,
}));

jest.mock('@/components/ui/ragflow-pagination', () => ({
  RAGFlowPagination: () => <button>Pagination</button>,
}));

import { DatasetTable } from './dataset-table';

const mockFormatBytes = jest.requireMock('@/utils/file-util')
  .formatBytes as jest.Mock;

describe('DatasetTable', () => {
  it('renders dataset creation time, total size and shared status counts', () => {
    render(
      <DatasetTable
        documents={[]}
        pagination={{ current: 1, pageSize: 10, total: 0 }}
        setPagination={jest.fn()}
        loading={false}
        rowSelection={{}}
        setRowSelection={jest.fn()}
        showManageMetadataModal={jest.fn()}
      />,
    );

    expect(screen.getByText(/common.createdAt/)).toHaveTextContent(
      `common.createdAt ${dayjs(mockKnowledgeBase.create_time).format('DD/MM/YYYY HH:mm:ss')}`,
    );
    expect(mockFormatBytes).toHaveBeenCalledWith(mockKnowledgeBase.size);
    expect(screen.getByText('1.5 GB')).toBeInTheDocument();
    expect(
      screen.getByText('knowledgeDetails.filesPending:2'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('knowledgeDetails.filesParsing:12'),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Pagination' }),
    ).toBeInTheDocument();
  });
});
