import { render, screen } from '@testing-library/react';
import React from 'react';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));

import ProcessLogModal from './process-log-modal';
import { TooltipProvider } from '@/components/ui/tooltip';
import { formatTime } from '@/utils/date';

const eventTs = '2026-09-18T10:20:30+08:00';

type LogEvent = {
  id: number;
  ts: string;
  event_type: number;
  component: string;
  phase: number;
  message: string;
};

function renderModal(events: LogEvent[]) {
  return render(
    React.createElement(
      TooltipProvider,
      null,
      React.createElement(ProcessLogModal, {
        visible: true,
        onCancel: jest.fn(),
        title: 'log',
        logInfo: {
          fileName: 'doc.pdf',
          details: '',
          events,
        },
      }),
    ),
  );
}

describe('ProcessLogModal ingestion events', () => {
  it('renders each event line prefixed with its timestamp', () => {
    renderModal([
      {
        id: 1,
        ts: eventTs,
        event_type: 0,
        component: 'Parser',
        phase: 1,
        message: 'Parser Done',
      },
      {
        id: 2,
        ts: '2026-09-18T10:21:05+08:00',
        event_type: 1,
        component: '',
        phase: 0,
        message: 'embedding 40/120 chunks',
      },
    ]);

    const times = screen.getAllByTestId('ingestion-event-time');
    expect(times).toHaveLength(2);
    expect(times[0].textContent).toBe(formatTime(eventTs));
    expect(screen.getByText('embedding 40/120 chunks')).toBeInTheDocument();
  });

  it('omits the timestamp span when ts is absent', () => {
    renderModal([
      {
        id: 3,
        ts: '',
        event_type: 0,
        component: 'Tokenizer',
        phase: 0,
        message: 'Tokenizer Started',
      },
    ]);

    expect(
      screen.queryByTestId('ingestion-event-time'),
    ).not.toBeInTheDocument();
    expect(screen.getByText('Tokenizer Started')).toBeInTheDocument();
  });
});
