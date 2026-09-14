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

// Drive both the useIsGoBackend hook and the pickByBackend payload/status
// adapters from a single mutable flag.
let mockIsGoBackend = false;
jest.mock('@/utils/backend-variant', () => ({
  useIsGoBackend: () => mockIsGoBackend,
  pickByBackend: ({ go, python }: { go: unknown; python: unknown }) =>
    mockIsGoBackend ? go : python,
}));

import { IngestionTaskStatus, RunningStatus } from './constant';
import { ParsingStatusCell } from './parsing-status-cell';

const baseRecord = {
  id: 'doc-1',
  dataset_id: 'dataset-1',
  name: 'doc.pdf',
  type: 'document',
  run: RunningStatus.UNSTART,
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
    const href =
      el.getAttribute('xlink:href') ?? el.getAttribute('href') ?? '';
    return href === `#icon-${name}`;
  });
}

describe('ParsingStatusCell', () => {
  beforeEach(() => {
    mockRunDocumentByIds.mockClear();
    mockShowReparseDialog.mockClear();
  });

  afterEach(() => {
    mockIsGoBackend = false;
  });

  it('shows a cancel icon while the legacy run state is RUNNING on Python', () => {
    const { container } = renderCell({ run: RunningStatus.RUNNING });

    expect(container.querySelector('svg.lucide-circle-x')).toBeInTheDocument();
    expect(hasIconFont(container, 'play')).toBe(false);
    // Python never reports STOPPING, so the loading mask must not render.
    expect(
      screen.queryByTestId('document-stopping-overlay'),
    ).not.toBeInTheDocument();
    expect(getSection(container)).toHaveAttribute('data-state', 'running');
  });

  it('shows the play action for an idle Python document', () => {
    const { container } = renderCell({ run: RunningStatus.UNSTART });

    expect(getSection(container)).toHaveAttribute('data-state', 'unstart');
    expect(hasIconFont(container, 'play')).toBe(true);
    expect(
      container.querySelector('svg.lucide-circle-x'),
    ).not.toBeInTheDocument();
  });

  describe('Go backend', () => {
    beforeEach(() => {
      mockIsGoBackend = true;
    });

    it('shows the queued chip for CREATED tasks', () => {
      const { container } = renderCell({
        run: undefined,
        ingestion_status: IngestionTaskStatus.CREATED,
      });
      expect(getSection(container)).toHaveAttribute('data-state', 'queued');
      expect(
        screen.getByText('knowledgeDetails.runningStatusQueued'),
      ).toBeInTheDocument();
    });

    it('shows progress and an enabled cancel button while RUNNING', () => {
      const { container } = renderCell({
        run: undefined,
        ingestion_status: IngestionTaskStatus.RUNNING,
      });
      expect(getSection(container)).toHaveAttribute('data-state', 'running');
      expect(container.querySelector('svg.lucide-circle-x')).toBeInTheDocument();
      expect(
        container.querySelector('button[disabled]'),
      ).not.toBeInTheDocument();
    });

    it('masks the progress area with a loading overlay while STOPPING', () => {
      const { container } = renderCell({
        run: undefined,
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

    it('renders the success state and reparse action for COMPLETED', () => {
      const { container } = renderCell({
        run: undefined,
        ingestion_status: IngestionTaskStatus.COMPLETED,
      });
      expect(getSection(container)).toHaveAttribute('data-state', 'success');
      expect(
        container.querySelector('svg.lucide-circle-x'),
      ).not.toBeInTheDocument();
    });

    it('renders the cancel state for STOPPED', () => {
      const { container } = renderCell({
        run: undefined,
        ingestion_status: IngestionTaskStatus.STOPPED,
      });
      expect(getSection(container)).toHaveAttribute('data-state', 'cancel');
    });

    it('renders the play action for UNSTART without a run field', () => {
      const { container } = renderCell({
        run: undefined,
        ingestion_status: IngestionTaskStatus.UNSTART,
      });
      expect(getSection(container)).toHaveAttribute('data-state', 'unstart');
      expect(hasIconFont(container, 'play')).toBe(true);
    });

    it('treats a missing ingestion_status as idle', () => {
      const { container } = renderCell({ run: undefined });
      expect(getSection(container)).toHaveAttribute('data-state', 'unstart');
      expect(
        container.querySelector('svg.lucide-circle-x'),
      ).not.toBeInTheDocument();
    });
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
