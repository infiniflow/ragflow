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

import { IngestionTaskStatus } from './constant';
import { ParsingStatusCell } from './parsing-status-cell';

const baseRecord = {
  id: 'doc-1',
  dataset_id: 'dataset-1',
  name: 'doc.pdf',
  type: 'document',
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

function renderCell(overrides: Record<string, any> = {}) {
  const record = { ...baseRecord, ...overrides } as any;
  return render(
    React.createElement(ParsingStatusCell, {
      record,
      showLog: jest.fn(),
      showChangeParserModal: jest.fn(),
    }),
    { wrapper: StrictMode },
  );
}

function getSection(container: HTMLElement) {
  return container.querySelector(
    '[data-testid="document-parse-status"]',
  ) as HTMLElement;
}

// IconFontFill renders <use xlink:href="#icon-<name>"/>; jsdom keeps the
// namespaced attribute, so read it instead of relying on a CSS selector.
function hasIconFont(container: HTMLElement, name: string) {
  return Array.from(container.querySelectorAll('svg use')).some((el) => {
    const href = el.getAttribute('xlink:href') ?? el.getAttribute('href') ?? '';
    return href === `#icon-${name}`;
  });
}

describe('ParsingStatusCell', () => {
  beforeEach(() => {
    mockRunDocumentByIds.mockClear();
    mockShowReparseDialog.mockClear();
  });

  it('shows the queued chip for CREATED tasks', () => {
    const { container } = renderCell({
      ingestion_status: IngestionTaskStatus.CREATED,
    });
    expect(getSection(container)).toHaveAttribute('data-state', 'queued');
    expect(
      screen.getByText('knowledgeDetails.runningStatusQueued'),
    ).toBeInTheDocument();
  });

  it('shows progress and an enabled cancel button while RUNNING', () => {
    const { container } = renderCell({
      ingestion_status: IngestionTaskStatus.RUNNING,
    });
    expect(getSection(container)).toHaveAttribute('data-state', 'running');
    expect(container.querySelector('svg.lucide-circle-x')).toBeInTheDocument();
    expect(container.querySelector('button[disabled]')).not.toBeInTheDocument();
  });

  it('masks the progress area with a loading overlay while STOPPING', () => {
    const { container } = renderCell({
      ingestion_status: IngestionTaskStatus.STOPPING,
    });
    expect(getSection(container)).toHaveAttribute('data-state', 'stopping');

    // The underlying running row stays visible but reads as disabled:
    // dimmed, flagged as stopping and non-interactive.
    const row = screen.getByTestId('document-processing-row');
    expect(row).toHaveAttribute('data-stopping');
    expect(row).toHaveClass('opacity-50');
    expect(row).toHaveClass('pointer-events-none');

    // The overlay communicates the in-flight cancel with a plain
    // spinner (no status text) and blocks the actions beneath.
    const overlay = screen.getByTestId('document-stopping-overlay');
    expect(overlay).toBeInTheDocument();
    expect(
      screen.queryByText('knowledgeDetails.runningStatusStopping'),
    ).not.toBeInTheDocument();
    expect(overlay.querySelector('.animate-spin')).toBeInTheDocument();

    // Both the progress/log button and the cancel button are disabled.
    const buttons = container.querySelectorAll('button');
    buttons.forEach((button) => expect(button).toBeDisabled());
  });

  it('renders the success state for COMPLETED', () => {
    const { container } = renderCell({
      ingestion_status: IngestionTaskStatus.COMPLETED,
    });
    expect(getSection(container)).toHaveAttribute('data-state', 'success');
    expect(
      container.querySelector('svg.lucide-circle-x'),
    ).not.toBeInTheDocument();
  });

  it('renders the cancel state for STOPPED', () => {
    const { container } = renderCell({
      ingestion_status: IngestionTaskStatus.STOPPED,
    });
    expect(getSection(container)).toHaveAttribute('data-state', 'cancel');
  });

  it('renders the play action for UNSTART', () => {
    const { container } = renderCell({
      ingestion_status: IngestionTaskStatus.UNSTART,
    });
    expect(getSection(container)).toHaveAttribute('data-state', 'unstart');
    expect(hasIconFont(container, 'play')).toBe(true);
  });

  it('treats a missing ingestion_status as idle', () => {
    const { container } = renderCell();
    expect(getSection(container)).toHaveAttribute('data-state', 'unstart');
    expect(
      container.querySelector('svg.lucide-circle-x'),
    ).not.toBeInTheDocument();
  });

  it('asks for confirmation before dropping existing chunks', () => {
    renderCell({
      ingestion_status: IngestionTaskStatus.COMPLETED,
      chunk_count: 3,
      parser_config: { enable_metadata: true },
    });

    fireEvent.click(screen.getByTestId('document-parse-toggle'));

    expect(mockShowReparseDialog).toHaveBeenCalledTimes(1);
    expect(mockRunDocumentByIds).not.toHaveBeenCalled();
  });

  it('re-parses a chunkless document straight away without confirmation', () => {
    renderCell({
      ingestion_status: IngestionTaskStatus.COMPLETED,
      chunk_count: 0,
      parser_config: { enable_metadata: true },
    });

    fireEvent.click(screen.getByTestId('document-parse-toggle'));

    expect(mockRunDocumentByIds).toHaveBeenCalledTimes(1);
    expect(mockShowReparseDialog).not.toHaveBeenCalled();
  });
});
