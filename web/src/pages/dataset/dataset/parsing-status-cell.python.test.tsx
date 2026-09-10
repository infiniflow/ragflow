import { render } from '@testing-library/react';
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
import { ParsingStatusCellPython } from './parsing-status-cell.python';

const baseRecord = {
  id: 'doc-1',
  dataset_id: 'dataset-1',
  name: 'doc.pdf',
  type: 'document',
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
  progress: 0,
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

describe('ParsingStatusCellPython', () => {
  it('shows progress while running', () => {
    const { container } = render(
      React.createElement(ParsingStatusCellPython, {
        record: { ...baseRecord, run: RunningStatus.RUNNING },
        showLog: jest.fn(),
      }),
    );

    expect(container.querySelector('svg.lucide-circle-x')).toBeInTheDocument();
    expect(
      container.querySelector('[data-testid="document-parse-status"]'),
    ).toHaveAttribute('data-state', 'running');
  });

  it('shows a reparse icon for terminal states', () => {
    const { container } = render(
      React.createElement(ParsingStatusCellPython, {
        record: { ...baseRecord, run: RunningStatus.DONE },
        showLog: jest.fn(),
      }),
    );

    expect(
      container.querySelector('use[xlink\\:href="#icon-reparse"]'),
    ).toBeInTheDocument();
  });

  it('ignores ingestion_status and renders from run only', () => {
    const { container } = render(
      React.createElement(ParsingStatusCellPython, {
        record: {
          ...baseRecord,
          run: RunningStatus.UNSTART,
          ingestion_status: IngestionTaskStatus.RUNNING,
        },
        showLog: jest.fn(),
      }),
    );

    expect(
      container.querySelector('use[xlink\\:href="#icon-play"]'),
    ).toBeInTheDocument();
    expect(
      container.querySelector('svg.lucide-circle-x'),
    ).not.toBeInTheDocument();
  });
});
