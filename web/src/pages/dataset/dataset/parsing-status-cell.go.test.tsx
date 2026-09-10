import { render, screen } from '@testing-library/react';
import React from 'react';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('./reparse-dialog', () => ({
  ReparseDialog: () => null,
}));

jest.mock('./use-run-document', () => ({
  useHandleRunDocumentByIds: () => ({
    handleRunDocumentByIds: jest.fn(),
    visible: false,
    showModal: jest.fn(),
    hideModal: jest.fn(),
  }),
}));

import { IngestionTaskStatus, RunningStatus } from './constant';
import { ParsingStatusCellGo } from './parsing-status-cell.go';

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

describe('ParsingStatusCellGo', () => {
  it('shows a cancel icon while ingestion runs before the document run state updates', () => {
    const { container } = render(
      React.createElement(ParsingStatusCellGo, {
        record: {
          ...baseRecord,
          run: RunningStatus.UNSTART,
          ingestion_status: IngestionTaskStatus.RUNNING,
        },
        showLog: jest.fn(),
        showChangeParserModal: jest.fn(),
      }),
    );

    expect(container.querySelector('svg.lucide-circle-x')).toBeInTheDocument();
    expect(
      container.querySelector('use[xlink\\:href="#icon-play"]'),
    ).not.toBeInTheDocument();
  });

  it('shows a queued state for enqueued ingestion tasks', () => {
    const { container } = render(
      React.createElement(ParsingStatusCellGo, {
        record: {
          ...baseRecord,
          run: RunningStatus.UNSTART,
          ingestion_status: IngestionTaskStatus.SCHEDULED,
        },
        showLog: jest.fn(),
        showChangeParserModal: jest.fn(),
      }),
    );

    expect(
      container.querySelector('[data-testid="document-parse-status"]'),
    ).toHaveAttribute('data-state', 'queued');
    expect(
      screen.queryByText('knowledgeDetails.runningStatusQueued'),
    ).toBeInTheDocument();
  });

  it('disables cancel while stopping', () => {
    const { container } = render(
      React.createElement(ParsingStatusCellGo, {
        record: {
          ...baseRecord,
          run: RunningStatus.RUNNING,
          ingestion_status: IngestionTaskStatus.STOPPING,
        },
        showLog: jest.fn(),
        showChangeParserModal: jest.fn(),
      }),
    );

    expect(
      container.querySelector('[data-testid="document-parse-status"]'),
    ).toHaveAttribute('data-state', 'running');
    expect(container.querySelector('svg.lucide-circle-x')).toBeInTheDocument();
    expect(container.querySelector('button[disabled]')).toBeInTheDocument();
  });
});
