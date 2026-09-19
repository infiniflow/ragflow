import request from '@/utils/request';
import { render, screen, waitFor } from '@testing-library/react';
import { AudioPreviewer } from './audio-preview';

jest.mock('@/utils/request', () => ({
  __esModule: true,
  default: jest.fn(),
}));

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { error: jest.fn() },
}));

jest.mock('@/components/ui/spin', () => ({
  Spin: () => <div data-testid="spin" />,
}));

const MockRequest = jest.mocked(request);

beforeAll(() => {
  URL.createObjectURL = jest.fn(() => 'blob:audio-mock');
  URL.revokeObjectURL = jest.fn();
});

afterAll(() => {
  jest.restoreAllMocks();
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
      { method: 'GET', responseType: 'blob', onError: expect.any(Function) },
    );
    expect(
      screen.getByTestId('document-audio-player').getAttribute('src'),
    ).toBe('blob:audio-mock');
  });

  it('renders no player when the request fails', async () => {
    MockRequest.mockImplementation(
      (_url: string, options: any) =>
        new Promise(() => {
          options.onError?.();
        }),
    );

    render(<AudioPreviewer url="/api/v1/agents/attachments/doc2/preview" />);

    await waitFor(() => expect(MockRequest).toHaveBeenCalled());
    expect(
      screen.queryByTestId('document-audio-player'),
    ).not.toBeInTheDocument();
  });
});
