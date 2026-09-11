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
import { ParsingStatusCell } from './parsing-status-cell';

describe('ParsingStatusCell', () => {
  it('shows a cancel icon while ingestion runs before the document run state updates', () => {
    const { container } = render(
      React.createElement(ParsingStatusCell, {
        record: {
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
        },
        showLog: jest.fn(),
        showChangeParserModal: jest.fn(),
      }),
    );

    expect(container.querySelector('svg.lucide-circle-x')).toBeInTheDocument();
    expect(
      container.querySelector('use[href="#icon-play"]'),
    ).not.toBeInTheDocument();
  });

  it('does not expose cancelling as a document status', () => {
    const { container } = render(
      React.createElement(ParsingStatusCell, {
        record: {
          id: 'doc-1',
          dataset_id: 'dataset-1',
          name: 'doc.pdf',
          type: 'document',
          run: RunningStatus.RUNNING,
          ingestion_status: IngestionTaskStatus.STOPPING,
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
        },
        showLog: jest.fn(),
        showChangeParserModal: jest.fn(),
      }),
    );

    expect(
      screen.queryByText('knowledgeDetails.runningStatusStopping'),
    ).not.toBeInTheDocument();
    expect(container.querySelector('svg.lucide-circle-x')).toBeInTheDocument();
    expect(container.querySelector('button[disabled]')).toBeInTheDocument();
  });
});
