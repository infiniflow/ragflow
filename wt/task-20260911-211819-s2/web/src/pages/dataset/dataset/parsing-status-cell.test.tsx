import { fireEvent, render, screen } from '@testing-library/react';
import React, { StrictMode } from 'react';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('./reparse-dialog', () => ({
  ReparseDialog: () => null,
}));

const mockRunDocumentByIds = jest.fn();
const mockShowReparseDialog = jest.fn();

jest.mock('./use-run-document', () => ({
  useHandleRunDocumentByIds: () => ({
    handleRunDocumentByIds: mockRunDocumentByIds,
    visible: false,
    showModal: mockShowReparseDialog,
    hideModal: jest.fn(),
  }),
}));

import { IngestionTaskStatus, RunningStatus } from './constant';
import { ParsingStatusCell } from './parsing-status-cell';

const baseRecord = {
  id: 'doc-1',
  dataset_id: 'dataset-1',
  name: 'doc.pdf',
  type: 'document',
  run: RunningStatus.UNSTART,
  ingestion_status: IngestionTaskStatus.RUNNING,
  progress: 0,
  chunk_count: 0,
  parser_config: {},
  create_date: '',
  create_time: 0,
  created_by: 'user-1',
  nickname: '',
  location: '',
  pipeline_id: '',
  pipeline_name: '',
  process_duration: 0,
  progress_msg: '',
  size: 0,
  source_type: 'local',
  status: '1',
  suffix: 'pdf',
  thumbnail: '',
  token_num: 0,
  update_date: '',
  update_time: 0,
  chunk_method: 'naive',
};

function renderCell(overrides: Record<string, any>) {
  return render(
    React.createElement(ParsingStatusCell, {
      record: { ...baseRecord, ...overrides } as any,
      showLog: jest.fn(),
      showChangeParserModal: jest.fn(),
    }),
    { wrapper: StrictMode },
  );
}

describe('ParsingStatusCell', () => {
  beforeEach(() => {
    mockRunDocumentByIds.mockClear();
    mockShowReparseDialog.mockClear();
  });

  it('shows a cancel icon while ingestion runs before the document run state updates', () => {
    const { container } = renderCell({
      ingestion_status: IngestionTaskStatus.RUNNING,
    });

    expect(container.querySelector('svg.lucide-circle-x')).toBeInTheDocument();
    expect(
      container.querySelector('use[href="#icon-play"]'),
    ).not.toBeInTheDocument();
  });

  it('does not expose cancelling as a document status', () => {
    const { container } = renderCell({
      run: RunningStatus.RUNNING,
      ingestion_status: IngestionTaskStatus.STOPPING,
    });

    expect(
      screen.queryByText('knowledgeDetails.runningStatusStopping'),
    ).not.toBeInTheDocument();
    expect(container.querySelector('svg.lucide-circle-x')).toBeInTheDocument();
    expect(container.querySelector('button[disabled]')).toBeInTheDocument();
  });

  it('runs a chunkless document once without opening the confirmation', () => {
    renderCell({ ingestion_status: IngestionTaskStatus.COMPLETED });

    fireEvent.click(screen.getByTestId('document-parse-toggle'));

    expect(mockRunDocumentByIds).toHaveBeenCalledTimes(1);
    expect(mockShowReparseDialog).not.toHaveBeenCalled();
  });

  it('opens the confirmation when existing chunks would be dropped', () => {
    renderCell({
      run: RunningStatus.DONE,
      ingestion_status: IngestionTaskStatus.COMPLETED,
      chunk_count: 3,
    });

    fireEvent.click(screen.getByTestId('document-parse-toggle'));

    expect(mockShowReparseDialog).toHaveBeenCalledTimes(1);
    expect(mockRunDocumentByIds).not.toHaveBeenCalled();
  });

  it('opens the confirmation for a chunkless document when auto-metadata applies', () => {
    renderCell({
      ingestion_status: IngestionTaskStatus.COMPLETED,
      parser_config: { enable_metadata: true },
    });

    fireEvent.click(screen.getByTestId('document-parse-toggle'));

    expect(mockShowReparseDialog).toHaveBeenCalledTimes(1);
    expect(mockRunDocumentByIds).not.toHaveBeenCalled();
  });
});
