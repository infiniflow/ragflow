import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { TooltipProvider } from '@/components/ui/tooltip';
import { MemoryRouter, useNavigate } from 'react-router';
import Files from './index';

const mockShowConnectToKnowledgeModal = jest.fn();

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/components/bulk-operate-bar', () => ({
  BulkOperateBar: ({
    list,
    count,
  }: {
    list: Array<{ id: string; onClick: () => void }>;
    count: number;
  }) => (
    <div data-testid="bulk-operate-bar" data-count={count}>
      {list.map((item) => (
        <button
          key={item.id}
          data-testid={`bulk-${item.id}`}
          onClick={item.onClick}
        >
          {item.id}
        </button>
      ))}
    </div>
  ),
}));

jest.mock('@/components/file-upload-dialog', () => ({
  FileUploadDialog: () => null,
}));

jest.mock('@/components/list-filter-bar', () => ({
  __esModule: true,
  default: ({
    children,
    leftPanel,
  }: {
    children?: React.ReactNode;
    leftPanel?: React.ReactNode;
  }) => (
    <div>
      {leftPanel}
      {children}
    </div>
  ),
}));

jest.mock('@/components/ui/ragflow-pagination', () => ({
  RAGFlowPagination: ({
    current = 1,
    pageSize = 10,
    onChange,
  }: {
    current?: number;
    pageSize?: number;
    onChange?: (page: number, pageSize: number) => void;
  }) => {
    const handleNextPage = () => onChange?.(current + 1, pageSize);

    return (
      <button data-testid="next-page" onClick={handleNextPage}>
        next page
      </button>
    );
  },
}));

jest.mock('@/hooks/logic-hooks', () => ({
  useGoToPreviousPageOnEmpty: jest.fn(),
}));

jest.mock('@/hooks/use-file-request', () => {
  const React = jest.requireActual('react');
  const { useSearchParams } = jest.requireActual('react-router');

  const file = (id: string, name: string, type = 'pdf') => ({
    id,
    name,
    type,
    source_type: '',
    create_time: 0,
    size: 1,
    kbs_info: [],
  });

  return {
    useFetchFileList: () => {
      const [searchParams] = useSearchParams();
      const folderId = searchParams.get('folderId') ?? '';
      const [pagination, setPagination] = React.useState({
        current: 1,
        pageSize: 10,
      });
      const page = pagination.current;
      const files =
        folderId === 'a'
          ? page === 1
            ? [file('a1', 'A file'), file('b', 'Folder B', 'folder')]
            : [file('a2', 'A second-page file')]
          : folderId === 'b'
            ? [file('b1', 'B file')]
            : [file('a', 'Folder A', 'folder')];

      return {
        files,
        total: 20,
        loading: false,
        pagination,
        setPagination: ({
          page,
          pageSize,
        }: {
          page: number;
          pageSize: number;
        }) => setPagination({ current: page, pageSize }),
        searchString: '',
        setSearchString: jest.fn(),
        handleInputChange: jest.fn(),
        folderNotFound: false,
      };
    },
    useFetchParentFolderList: () => {
      const [searchParams] = useSearchParams();
      const folderId = searchParams.get('folderId') ?? '';
      if (!folderId) return [];
      return [
        { id: '', name: '/' },
        ...(folderId === 'b' ? [{ id: 'a', name: 'Folder A' }] : []),
        { id: folderId, name: `Folder ${folderId.toUpperCase()}` },
      ];
    },
    useDownloadFile: () => ({ downloadFile: jest.fn() }),
  };
});

jest.mock('./action-cell', () => ({ ActionCell: () => null }));
jest.mock('./knowledge-cell', () => ({ KnowledgeCell: () => null }));
jest.mock('./link-to-dataset-dialog', () => ({
  LinkToDatasetDialog: () => null,
}));
jest.mock('./move-dialog', () => ({ MoveDialog: () => null }));
jest.mock('./create-folder-dialog', () => ({
  CreateFolderDialog: () => null,
}));

jest.mock('./hooks', () => {
  const { useSearchParams } = jest.requireActual('react-router');

  return {
    useGetFolderId: () => {
      const [searchParams] = useSearchParams();
      return searchParams.get('folderId') ?? '';
    },
    useHandleConnectToKnowledge: () => ({
      connectToKnowledgeVisible: false,
      hideConnectToKnowledgeModal: jest.fn(),
      showConnectToKnowledgeModal: mockShowConnectToKnowledgeModal,
      initialConnectedIds: [],
      onConnectToKnowledgeOk: jest.fn(),
      connectToKnowledgeLoading: false,
    }),
    useRenameCurrentFile: () => ({
      fileRenameVisible: false,
      showFileRenameModal: jest.fn(),
      hideFileRenameModal: jest.fn(),
      onFileRenameOk: jest.fn(),
      initialFileName: '',
      fileRenameLoading: false,
    }),
  };
});

jest.mock('./use-delete-file', () => ({
  useHandleDeleteFile: () => ({ handleRemoveFile: jest.fn() }),
}));

jest.mock('./use-upload-file', () => ({
  useHandleUploadFile: () => ({
    fileUploadVisible: false,
    hideFileUploadModal: jest.fn(),
    showFileUploadModal: jest.fn(),
    fileUploadLoading: false,
    onFileUploadOk: jest.fn(),
  }),
}));

jest.mock('./use-create-folder', () => ({
  useHandleCreateFolder: () => ({
    folderCreateModalVisible: false,
    showFolderCreateModal: jest.fn(),
    hideFolderCreateModal: jest.fn(),
    folderCreateLoading: false,
    onFolderCreateOk: jest.fn(),
  }),
}));

jest.mock('./use-move-file', () => ({
  useHandleMoveFile: () => ({
    showMoveFileModal: jest.fn(),
    moveFileVisible: false,
    onMoveFileOk: jest.fn(),
    hideMoveFileModal: jest.fn(),
    moveFileLoading: false,
  }),
}));

function HistoryControls() {
  const navigate = useNavigate();
  const goBack = () => navigate(-1);
  const goForward = () => navigate(1);

  return (
    <>
      <button onClick={goBack}>back</button>
      <button onClick={goForward}>forward</button>
    </>
  );
}

function renderFiles(initialEntries = ['/files?folderId=a']) {
  return render(
    <MemoryRouter initialEntries={initialEntries}>
      <TooltipProvider>
        <HistoryControls />
        <Files />
      </TooltipProvider>
    </MemoryRouter>,
  );
}

function selectFile(name: string) {
  const row = screen.getByText(name).closest('tr');
  if (!row) throw new Error(`Missing row for ${name}`);
  fireEvent.click(within(row).getByLabelText('Select row'));
}

beforeEach(() => {
  mockShowConnectToKnowledgeModal.mockClear();
});

describe('Files selection scope', () => {
  it('drops old file IDs when a folder row opens another folder', async () => {
    renderFiles();
    selectFile('A file');

    fireEvent.click(screen.getByText('Folder B'));
    await screen.findByText('B file');
    selectFile('B file');
    fireEvent.click(screen.getByTestId('bulk-link'));

    expect(mockShowConnectToKnowledgeModal).toHaveBeenCalledWith(['b1']);
  });

  it('clears selection when a breadcrumb changes folders', async () => {
    renderFiles();
    selectFile('A file');

    fireEvent.click(screen.getByText('root'));
    await screen.findByText('Folder A');

    await waitFor(() => {
      expect(screen.queryByTestId('bulk-operate-bar')).not.toBeInTheDocument();
    });
  });

  it('clears selection on browser back and forward navigation', async () => {
    renderFiles();
    selectFile('A file');
    fireEvent.click(screen.getByText('Folder B'));
    await screen.findByText('B file');
    selectFile('B file');

    fireEvent.click(screen.getByText('back'));
    await screen.findByText('A file');
    await waitFor(() => {
      expect(screen.queryByTestId('bulk-operate-bar')).not.toBeInTheDocument();
    });

    selectFile('A file');
    fireEvent.click(screen.getByText('forward'));
    await screen.findByText('B file');
    await waitFor(() => {
      expect(screen.queryByTestId('bulk-operate-bar')).not.toBeInTheDocument();
    });
  });

  it('keeps clearing selection when pagination changes in one folder', async () => {
    renderFiles();
    selectFile('A file');

    fireEvent.click(screen.getByTestId('next-page'));
    await screen.findByText('A second-page file');

    await waitFor(() => {
      expect(screen.queryByTestId('bulk-operate-bar')).not.toBeInTheDocument();
    });
  });
});
