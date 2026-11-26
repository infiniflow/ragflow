import { fireEvent, render, screen, within } from '@testing-library/react';
import React, { StrictMode } from 'react';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/locales/config', () => ({
  __esModule: true,
  default: { t: (key: string) => key },
}));

jest.mock('./reparse-dialog', () => ({
  ReparseDialog: () => null,
}));

const mockRunDocumentByIds = jest.fn();
const mockShowReparseDialog = jest.fn();
const mockUseDocumentPipelineVersion = jest.fn();
const mockWarning = jest.fn();

jest.mock('./use-run-document', () => ({
  useHandleRunDocumentByIds: () => ({
    handleRunDocumentByIds: mockRunDocumentByIds,
    visible: false,
    showModal: mockShowReparseDialog,
    hideModal: jest.fn(),
  }),
}));

jest.mock('./use-document-pipeline-version', () => ({
  useDocumentPipelineVersion: (record: unknown) =>
    mockUseDocumentPipelineVersion(record),
}));

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { warning: (msg: string) => mockWarning(msg) },
}));

import { IngestionTaskStatus } from './constant';
import {
  ParseCell,
  ParseDropdownButton,
  ParsingStatusCell,
} from './parsing-status-cell';
import { TooltipProvider } from '@/components/ui/tooltip';

const baseRecord = {
  operator_permission: 1,
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

function createRecord(overrides: Record<string, any> = {}) {
  return { ...baseRecord, ...overrides } as any;
}

const mockShowLog = jest.fn();
const mockShowChangeParserModal = jest.fn();

function renderCell(overrides: Record<string, any> = {}) {
  return render(
    React.createElement(ParsingStatusCell, {
      record: createRecord(overrides),
      showLog: mockShowLog,
    }),
    { wrapper: StrictMode },
  );
}

function renderParseCell(overrides: Record<string, any> = {}) {
  return render(
    React.createElement(
      TooltipProvider,
      null,
      React.createElement(ParseCell, {
        record: createRecord(overrides),
        showChangeParserModal: mockShowChangeParserModal,
      }),
    ),
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
    mockWarning.mockClear();
    mockShowLog.mockClear();
    mockShowChangeParserModal.mockClear();
    mockUseDocumentPipelineVersion.mockReset();
    mockUseDocumentPipelineVersion.mockReturnValue({
      version: undefined,
      missing: false,
    });
  });

  it('shows the queued chip for CREATED tasks', () => {
    const { container } = renderCell({
      ingestion_status: IngestionTaskStatus.CREATED,
    });
    expect(getSection(container)).toHaveAttribute('data-state', 'queued');
    expect(
      screen.getByText('knowledgeDetails.runningStatusQueued'),
    ).toBeInTheDocument();

    const { container: parseContainer } = renderParseCell({
      ingestion_status: IngestionTaskStatus.CREATED,
    });
    expect(
      parseContainer.querySelector('svg.lucide-circle-x'),
    ).toBeInTheDocument();
  });

  it('shows progress in Status and an enabled cancel button in Parse while RUNNING', () => {
    const { container } = renderCell({
      ingestion_status: IngestionTaskStatus.RUNNING,
    });
    expect(getSection(container)).toHaveAttribute('data-state', 'running');
    expect(container.querySelector('button[disabled]')).not.toBeInTheDocument();

    const { container: parseContainer } = renderParseCell({
      ingestion_status: IngestionTaskStatus.RUNNING,
    });
    expect(
      parseContainer.querySelector('svg.lucide-circle-x'),
    ).toBeInTheDocument();
    expect(
      within(parseContainer).getByTestId('document-parse-toggle'),
    ).toBeEnabled();
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

    const { container: parseContainer } = renderParseCell({
      ingestion_status: IngestionTaskStatus.STOPPING,
    });
    expect(
      within(parseContainer).getByTestId('document-parse-toggle'),
    ).toBeDisabled();
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

    const { container: parseContainer } = renderParseCell({
      ingestion_status: IngestionTaskStatus.UNSTART,
    });
    expect(hasIconFont(parseContainer, 'play')).toBe(true);
  });

  it('treats a missing ingestion_status as idle', () => {
    const { container } = renderCell();
    expect(getSection(container)).toHaveAttribute('data-state', 'unstart');
    expect(
      container.querySelector('svg.lucide-circle-x'),
    ).not.toBeInTheDocument();
  });

  it('shows View error for FAILED and opens the document log', () => {
    const { container } = renderCell({
      ingestion_status: IngestionTaskStatus.FAILED,
    });

    expect(getSection(container)).toHaveAttribute('data-state', 'fail');
    expect(screen.getByText('knowledgeDetails.viewError')).toBeVisible();
    expect(container.querySelector('svg.lucide-triangle-alert')).toBeVisible();
    fireEvent.click(screen.getByTestId('document-view-error'));

    expect(mockShowLog).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'doc-1' }),
    );
  });

  it.each([undefined, 'parser_error', 'license_required'])(
    'keeps View error for an unclassified failure (%s)',
    (errorCode) => {
      renderCell({
        ingestion_status: IngestionTaskStatus.FAILED,
        latest_ingestion_event: {
          error_code: errorCode,
          message: 'feature is not enabled by the license',
        },
      });

      expect(screen.getByText('knowledgeDetails.viewError')).toBeVisible();
      expect(
        screen.queryByText('knowledgeDetails.licenseRequired'),
      ).not.toBeInTheDocument();
    },
  );

  it.each([
    IngestionTaskStatus.CREATED,
    IngestionTaskStatus.RUNNING,
    IngestionTaskStatus.COMPLETED,
  ])('ignores a stale license error while %s', (ingestionStatus) => {
    renderCell({
      ingestion_status: ingestionStatus,
      latest_ingestion_event: { error_code: 'license_required' },
    });

    expect(screen.queryByTestId('document-view-error')).not.toBeInTheDocument();
    expect(
      screen.queryByText('knowledgeDetails.licenseRequired'),
    ).not.toBeInTheDocument();
  });

  it('opens the document log from a non-failure status dot', () => {
    renderCell({ ingestion_status: IngestionTaskStatus.COMPLETED });

    fireEvent.click(screen.getByRole('button'));

    expect(mockShowLog).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'doc-1' }),
    );
  });

  it('asks for confirmation before dropping existing chunks', () => {
    renderParseCell({
      ingestion_status: IngestionTaskStatus.COMPLETED,
      chunk_count: 3,
      parser_config: { enable_metadata: true },
    });

    fireEvent.click(screen.getByTestId('document-parse-toggle'));

    expect(mockShowReparseDialog).toHaveBeenCalledTimes(1);
    expect(mockRunDocumentByIds).not.toHaveBeenCalled();
  });

  it('re-parses a chunkless document straight away without confirmation', () => {
    renderParseCell({
      ingestion_status: IngestionTaskStatus.COMPLETED,
      chunk_count: 0,
      parser_config: { enable_metadata: true },
    });

    fireEvent.click(screen.getByTestId('document-parse-toggle'));

    expect(mockRunDocumentByIds).toHaveBeenCalledTimes(1);
    expect(mockShowReparseDialog).not.toHaveBeenCalled();
  });

  // The outdated bang (design D1/D2/D3) lives in the parser dropdown button,
  // so it is covered here against ParseDropdownButton.
  function renderDropdown(overrides: Record<string, any> = {}) {
    const record = { ...baseRecord, ...overrides } as any;
    return render(
      React.createElement(
        TooltipProvider,
        null,
        React.createElement(ParseDropdownButton, {
          record,
          showChangeParserModal: jest.fn(),
        }),
      ),
      { wrapper: StrictMode },
    );
  }

  it('labels a pipeline document by its version title and alias', () => {
    mockUseDocumentPipelineVersion.mockReturnValue({
      version: {
        id: 'v1',
        title: 'geek pipeline 2026-10-29 11:47:29',
        alias: '111',
      },
      missing: false,
    });
    renderDropdown({
      pipeline_id: 'canvas-1',
      pipeline_name: 'My pipeline',
      pipeline_version_id: 'v1',
    });

    expect(
      screen.getByText('geek pipeline 2026-10-29 11:47:29'),
    ).toBeInTheDocument();
    expect(screen.getByText('111')).toBeInTheDocument();
    expect(screen.queryByText('My pipeline')).not.toBeInTheDocument();
  });

  it('falls back to the pipeline name while the version is unresolved', () => {
    renderDropdown({
      pipeline_id: 'canvas-1',
      pipeline_name: 'My pipeline',
      pipeline_version_id: 'v1',
    });

    expect(screen.getByText('My pipeline')).toBeInTheDocument();
  });

  it('keeps the plain parser label for built-in parsers', () => {
    renderDropdown({ pipeline_id: '', chunk_method: 'naive' });

    expect(screen.getByText('General')).toBeInTheDocument();
  });

  it('shows the outdated bang when the pinned pipeline version is not the latest', () => {
    renderDropdown({
      pipeline_id: 'canvas-1',
      pipeline_version_outdated: true,
    });
    const bang = screen.getByText('!');
    expect(bang).toBeInTheDocument();
    expect(bang).toHaveClass('text-destructive');
  });

  it('hides the outdated bang for up-to-date or unpinned documents', () => {
    renderDropdown({
      pipeline_id: 'canvas-1',
      pipeline_version_outdated: false,
    });
    expect(screen.queryByText('!')).not.toBeInTheDocument();

    renderDropdown({ pipeline_id: '', pipeline_version_outdated: false });
    expect(screen.queryByText('!')).not.toBeInTheDocument();
  });

  it('shows the unavailable display when the bound pipeline version was deleted', () => {
    mockUseDocumentPipelineVersion.mockReturnValue({
      version: undefined,
      missing: true,
    });
    renderDropdown({
      pipeline_id: 'canvas-1',
      pipeline_name: 'My pipeline',
      pipeline_version_id: 'v1',
    });

    expect(
      screen.getByText('knowledgeConfiguration.parserOptionUnavailable'),
    ).toBeInTheDocument();
    expect(screen.queryByText('My pipeline')).not.toBeInTheDocument();
  });

  // A deleted pin is the stronger signal, so the outdated bang stays hidden
  // while the binding is missing (same precedence as ParserTreeSelect).
  it('hides the outdated bang while the binding is missing', () => {
    mockUseDocumentPipelineVersion.mockReturnValue({
      version: undefined,
      missing: true,
    });
    renderDropdown({
      pipeline_id: 'canvas-1',
      pipeline_version_id: 'v1',
      pipeline_version_outdated: true,
    });

    expect(screen.queryByText('!')).not.toBeInTheDocument();
  });

  it('warns and blocks parsing when the bound pipeline version was deleted', () => {
    mockUseDocumentPipelineVersion.mockReturnValue({
      version: undefined,
      missing: true,
    });
    renderParseCell({
      pipeline_id: 'canvas-1',
      pipeline_version_id: 'v1',
      ingestion_status: IngestionTaskStatus.COMPLETED,
      chunk_count: 3,
    });

    fireEvent.click(screen.getByTestId('document-parse-toggle'));

    expect(mockWarning).toHaveBeenCalledWith(
      'knowledgeConfiguration.parserOptionUnavailableMessage',
    );
    expect(mockRunDocumentByIds).not.toHaveBeenCalled();
    expect(mockShowReparseDialog).not.toHaveBeenCalled();
  });

  // Cancelling never touches the pipeline, so a missing binding must not
  // block stopping a running parse.
  it('still cancels a running parse when the binding is missing', () => {
    mockUseDocumentPipelineVersion.mockReturnValue({
      version: undefined,
      missing: true,
    });
    renderParseCell({
      pipeline_id: 'canvas-1',
      pipeline_version_id: 'v1',
      ingestion_status: IngestionTaskStatus.RUNNING,
    });

    fireEvent.click(screen.getByTestId('document-parse-toggle'));

    expect(mockWarning).not.toHaveBeenCalled();
    expect(mockRunDocumentByIds).toHaveBeenCalledTimes(1);
  });
});
