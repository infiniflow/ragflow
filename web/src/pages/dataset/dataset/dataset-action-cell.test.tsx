import { fireEvent, render, screen, waitFor } from '@testing-library/react';

const mockSetDocumentStatus = jest.fn();
const mockRemoveDocument = jest.fn();
const mockDownloadDatasetDocument = jest.fn();
const mockDownloadFileFromBlob = jest.fn();
const mockShowRenameModal = jest.fn();
const mockSetRowSelection = jest.fn();

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/use-document-request', () => ({
  useSetDocumentStatus: () => ({
    setDocumentStatus: mockSetDocumentStatus,
    loading: false,
  }),
  useRemoveDocument: () => ({ removeDocument: mockRemoveDocument }),
}));

jest.mock('@/services/file-manager-service', () => ({
  downloadDatasetDocument: (...args: unknown[]) =>
    mockDownloadDatasetDocument(...args),
}));

jest.mock('@/utils/file-util', () => ({
  downloadFileFromBlob: (...args: unknown[]) =>
    mockDownloadFileFromBlob(...args),
}));

jest.mock('@/ee/pages/dataset/dataset/dataset-action-cell-seams', () => ({
  DocumentPrivilegeAction: () => null,
  useDocumentActionCellOverrides: () => ({}),
}));

import { DocumentType } from '@/constants/knowledge';
import { IngestionTaskStatus } from './constant';
import { DatasetActionCell } from './dataset-action-cell';

const baseRecord = {
  id: 'doc-1',
  dataset_id: 'dataset-1',
  name: 'doc.pdf',
  type: 'document',
  status: '1',
  size: 100,
  create_time: 0,
  update_time: 0,
  ingestion_status: IngestionTaskStatus.COMPLETED,
};

beforeAll(() => {
  // jsdom lacks APIs Radix Popper/Menu rely on.
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  if (!Element.prototype.hasPointerCapture) {
    Element.prototype.hasPointerCapture = () => false;
  }
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {};
  }
});

beforeEach(() => {
  jest.clearAllMocks();
  mockSetDocumentStatus.mockResolvedValue({ code: 0 });
  mockRemoveDocument.mockResolvedValue(0);
  mockDownloadDatasetDocument.mockResolvedValue({
    data: new Blob(['content'], { type: 'application/pdf' }),
  });
});

function renderAction(overrides: Record<string, unknown> = {}) {
  const record = { ...baseRecord, ...overrides } as any;
  return render(
    <DatasetActionCell
      record={record}
      showRenameModal={mockShowRenameModal}
      setRowSelection={mockSetRowSelection}
    />,
  );
}

function openMenu() {
  // Radix DropdownMenuTrigger toggles on pointerdown and on ArrowDown/Enter.
  // jsdom pointer events do not reliably drive Radix, so open via keyboard.
  fireEvent.keyDown(
    screen.getByRole('button', { name: 'knowledgeDetails.action' }),
    { key: 'ArrowDown' },
  );
}

describe('DatasetActionCell', () => {
  it('disables an enabled document from the switch without closing the menu', async () => {
    renderAction();
    openMenu();

    expect(
      await screen.findByText('knowledgeDetails.enabledStatus'),
    ).toBeInTheDocument();
    const statusSwitch = screen.getByRole('switch');
    expect(statusSwitch).toBeChecked();
    fireEvent.click(statusSwitch);

    expect(mockSetDocumentStatus).toHaveBeenCalledWith({
      status: false,
      documentId: 'doc-1',
      datasetId: 'dataset-1',
    });
    expect(screen.getByRole('menu')).toBeInTheDocument();
  });

  it('enables a disabled document from the switch', async () => {
    renderAction({ status: '0' });
    openMenu();

    expect(
      await screen.findByText('knowledgeDetails.disabledStatus'),
    ).toBeInTheDocument();
    const statusSwitch = screen.getByRole('switch');
    expect(statusSwitch).not.toBeChecked();
    fireEvent.click(statusSwitch);

    expect(mockSetDocumentStatus).toHaveBeenCalledWith({
      status: true,
      documentId: 'doc-1',
      datasetId: 'dataset-1',
    });
  });

  it('opens rename from the menu', async () => {
    renderAction();
    openMenu();

    fireEvent.click(await screen.findByText('common.rename'));

    expect(mockShowRenameModal).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'doc-1' }),
    );
  });

  it('keeps the menu open while showing document info on focus', async () => {
    renderAction();
    openMenu();

    const infoItem = await screen.findByText('knowledgeDetails.info');
    fireEvent.focus(infoItem);

    expect(await screen.findByText('doc.pdf')).toBeInTheDocument();
    expect(screen.getByRole('menu')).toBeInTheDocument();
  });

  it('downloads the original document', async () => {
    renderAction();
    openMenu();

    fireEvent.click(await screen.findByText('common.download'));

    await waitFor(() =>
      expect(mockDownloadDatasetDocument).toHaveBeenCalledWith({
        datasetId: 'dataset-1',
        docId: 'doc-1',
        ext: 'pdf',
      }),
    );
    await waitFor(() =>
      expect(mockDownloadFileFromBlob).toHaveBeenCalledWith(
        expect.any(Blob),
        'doc.pdf',
      ),
    );
  });

  it('confirms deletion and prunes the deleted row from selection', async () => {
    renderAction();
    openMenu();

    fireEvent.click(await screen.findByTestId('document-delete'));
    fireEvent.click(
      await screen.findByTestId('confirm-delete-dialog-confirm-btn'),
    );

    await waitFor(() =>
      expect(mockRemoveDocument).toHaveBeenCalledWith('doc-1'),
    );
    await waitFor(() => expect(mockSetRowSelection).toHaveBeenCalledTimes(1));
    const updateSelection = mockSetRowSelection.mock.calls[0][0];
    expect(updateSelection({ 'doc-1': true, 'doc-2': true })).toEqual({
      'doc-2': true,
    });
  });

  it('hides download for virtual documents', async () => {
    renderAction({ type: DocumentType.Virtual });
    openMenu();

    expect(
      await screen.findByText('knowledgeDetails.info'),
    ).toBeInTheDocument();
    expect(screen.queryByText('common.download')).not.toBeInTheDocument();
  });

  it('keeps the existing running-document action guards', async () => {
    renderAction({ ingestion_status: IngestionTaskStatus.RUNNING });
    openMenu();

    await screen.findByRole('menu');
    for (const label of [
      'common.rename',
      'knowledgeDetails.info',
      'common.download',
      'common.delete',
    ]) {
      expect(
        screen.getByText(label).closest('[role="menuitem"]'),
      ).toHaveAttribute('aria-disabled', 'true');
    }
    expect(screen.getByRole('switch')).toBeEnabled();
  });
});
