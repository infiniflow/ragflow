import request from '@/utils/request';
import { render, screen, waitFor } from '@testing-library/react';
import { AudioPreviewer } from './audio-preview';

jest.mock('@/utils/request', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('@/components/ui/spin', () => ({
  Spin: () => <div data-testid="spin" />,
}));

const MockRequest = jest.mocked(request);

beforeAll(() => {
  URL.createObjectURL = jest.fn(
    (blob: Blob | MediaSource) => `blob:${(blob as Blob).type}`,
  );
  URL.revokeObjectURL = jest.fn();
});

afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  jest.clearAllMocks();
});

describe('AudioPreviewer', () => {
  it('fetches the audio through the authenticated url and renders a player', async () => {
    MockRequest.mockResolvedValue({
      data: new Blob([new Uint8Array([1, 2, 3])], { type: 'audio/mpeg' }),
    });

    render(<AudioPreviewer url="/api/v1/agents/attachments/doc1/preview" />);

    await waitFor(() =>
      expect(screen.getByTestId('document-audio-player')).toBeInTheDocument(),
    );
    expect(MockRequest).toHaveBeenCalledWith(
      '/api/v1/agents/attachments/doc1/preview',
      { method: 'GET', responseType: 'blob' },
    );
    expect(
      screen.getByTestId('document-audio-player').getAttribute('src'),
    ).toBe('blob:audio/mpeg');
  });

  it('renders an error state and no player when the response carries no blob', async () => {
    MockRequest.mockResolvedValue({ data: { code: 1999 } });

    render(<AudioPreviewer url="/api/v1/agents/attachments/doc2/preview" />);

    await waitFor(() =>
      expect(screen.getByTestId('document-audio-error')).toBeInTheDocument(),
    );
    expect(
      screen.queryByTestId('document-audio-player'),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId('spin')).not.toBeInTheDocument();
    expect(URL.createObjectURL).not.toHaveBeenCalled();
  });

  it('ignores a stale response when the url changes mid-flight', async () => {
    const pending: Record<string, (value: { data: Blob }) => void> = {};
    MockRequest.mockImplementation(
      (url: unknown) =>
        new Promise((resolve) => {
          pending[url as string] = resolve;
        }),
    );

    const oldUrl = '/api/v1/agents/attachments/old/preview';
    const newUrl = '/api/v1/agents/attachments/new/preview';
    const { rerender } = render(<AudioPreviewer url={oldUrl} />);
    rerender(<AudioPreviewer url={newUrl} />);

    pending[oldUrl]({
      data: new Blob([new Uint8Array([1])], { type: 'audio/old' }),
    });

    expect(
      screen.queryByTestId('document-audio-player'),
    ).not.toBeInTheDocument();
    expect(URL.createObjectURL).not.toHaveBeenCalled();

    pending[newUrl]({
      data: new Blob([new Uint8Array([2])], { type: 'audio/new' }),
    });

    await waitFor(() =>
      expect(screen.getByTestId('document-audio-player')).toBeInTheDocument(),
    );
    expect(
      screen.getByTestId('document-audio-player').getAttribute('src'),
    ).toBe('blob:audio/new');
  });
});
