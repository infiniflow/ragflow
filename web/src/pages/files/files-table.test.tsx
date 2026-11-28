import { fireEvent, render, screen } from '@testing-library/react';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/logic-hooks/navigate-hooks', () => ({
  useNavigatePage: () => ({ navigateToSkills: jest.fn() }),
}));

jest.mock('@/components/icon-font', () => ({
  FileIcon: () => null,
}));

jest.mock('@/components/rename-dialog', () => ({
  RenameDialog: () => null,
}));

jest.mock('@/components/table-skeleton', () => ({
  TableEmpty: ({ columnsLength }: { columnsLength: number }) => (
    <tr>
      <td colSpan={columnsLength}>Empty</td>
    </tr>
  ),
  TableSkeleton: ({ columnsLength }: { columnsLength: number }) => (
    <tr>
      <td colSpan={columnsLength}>Loading</td>
    </tr>
  ),
}));

jest.mock('@/components/ui/ragflow-pagination', () => ({
  RAGFlowPagination: ({
    onChange,
  }: {
    onChange: (page: number, pageSize: number) => void;
  }) => {
    const handleClick = () => onChange(2, 20);
    return <button onClick={handleClick}>Pagination</button>;
  },
}));

jest.mock('./action-cell', () => ({ ActionCell: () => null }));
jest.mock('./knowledge-cell', () => ({ KnowledgeCell: () => null }));
jest.mock('./link-to-dataset-dialog', () => ({
  LinkToDatasetDialog: () => null,
}));
jest.mock('./use-navigate-to-folder', () => ({
  useNavigateToOtherFolder: () => jest.fn(),
}));
jest.mock('./hooks', () => ({
  useRenameCurrentFile: () => ({
    fileRenameVisible: false,
    showFileRenameModal: jest.fn(),
    hideFileRenameModal: jest.fn(),
    onFileRenameOk: jest.fn(),
    initialFileName: '',
    fileRenameLoading: false,
  }),
}));

import { FilesTable } from './files-table';

const connectKnowledgeModal = {
  connectToKnowledgeVisible: false,
  hideConnectToKnowledgeModal: jest.fn(),
  showConnectToKnowledgeModal: jest.fn(),
  initialConnectedIds: [],
  onConnectToKnowledgeOk: jest.fn(),
  connectToKnowledgeLoading: false,
};

function renderFilesTable(createTime?: string) {
  const setPagination = jest.fn();
  render(
    <FilesTable
      files={[]}
      total={0}
      pagination={{ current: 1, pageSize: 10, total: 0 }}
      setPagination={setPagination}
      loading={false}
      rowSelection={{}}
      setRowSelection={jest.fn()}
      showMoveFileModal={jest.fn()}
      connectKnowledgeModal={connectKnowledgeModal}
      createTime={createTime}
    />,
  );
  return setPagination;
}

describe('FilesTable', () => {
  it('renders only current-folder metadata before functional pagination', () => {
    const setPagination = renderFilesTable('2025-03-14 14:53:39');

    const metadata = screen.getByText(/common.createdAt/);
    expect(metadata).toHaveTextContent('common.createdAt 2025-03-14 14:53:39');
    const pagination = screen.getByRole('button', { name: 'Pagination' });
    const footer = pagination.parentElement?.parentElement;
    expect(footer?.firstElementChild).toContainElement(metadata);
    expect(footer?.lastElementChild).toContainElement(pagination);
    expect(footer).not.toHaveTextContent(/files pending|files parsing|GB/);

    fireEvent.click(pagination);
    expect(setPagination).toHaveBeenCalledWith({ page: 2, pageSize: 20 });
  });

  it('does not render an empty creation-time label', () => {
    renderFilesTable();

    expect(screen.queryByText(/common.createdAt/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Pagination' })).toBeVisible();
  });
});
