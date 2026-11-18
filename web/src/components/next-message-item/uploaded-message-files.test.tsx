import { fireEvent, render, screen } from '@testing-library/react';
import { UploadedMessageFiles } from './uploaded-message-files';
import { isAudioFile } from './utils';

jest.mock('../file-icon', () => ({
  __esModule: true,
  default: ({ name }: { name: string }) => (
    <div data-testid="file-icon">{name}</div>
  ),
}));

const mockUseAuthenticatedImageUrl = jest.fn(
  (
    url: string | null | undefined,
  ): {
    src: string;
    status: 'loading' | 'ready' | 'error';
  } => ({
    src: url ?? '',
    status: 'ready',
  }),
);

jest.mock('../image', () => ({
  __esModule: true,
  useAuthenticatedImageUrl: (url: string | null | undefined) =>
    mockUseAuthenticatedImageUrl(url),
}));

jest.mock('../svg-icon', () => ({
  __esModule: true,
  default: ({ name }: { name: string }) => (
    <div data-testid="svg-icon">{name}</div>
  ),
}));

jest.mock('../ui/modal/modal', () => ({
  Modal: ({ open, title, children }: any) =>
    open ? (
      <div data-testid="audio-modal">
        <div data-testid="audio-modal-title">{title}</div>
        {children}
      </div>
    ) : null,
}));

jest.mock('../ui/spin', () => ({
  Spin: () => <div data-testid="spin" />,
}));

jest.mock('react-photo-view', () => ({
  PhotoProvider: ({ children }: any) => <>{children}</>,
  PhotoView: ({ children }: any) => <>{children}</>,
}));

const uploadedAudio = {
  created_at: 1789717195,
  created_by: 'tenant-1',
  extension: 'mp3',
  id: 'attach-1',
  mime_type: 'audio/mpeg',
  name: '1.01a demo reading.mp3',
  preview_url: null,
  size: 1024,
};

beforeAll(() => {
  URL.createObjectURL = jest.fn(() => 'blob:local-audio');
  URL.revokeObjectURL = jest.fn();
});

describe('UploadedMessageFiles audio playback', () => {
  it('detects audio files by mime type and by extension', () => {
    expect(isAudioFile(uploadedAudio)).toBe(true);
    expect(
      isAudioFile({
        ...uploadedAudio,
        mime_type: '',
        name: 'lesson.wav',
      }),
    ).toBe(true);
    expect(
      isAudioFile({
        ...uploadedAudio,
        mime_type: 'application/pdf',
        name: 'lesson.pdf',
      }),
    ).toBe(false);
  });

  it('opens an inline player when an uploaded audio chip is clicked', () => {
    render(
      <UploadedMessageFiles files={[uploadedAudio]}></UploadedMessageFiles>,
    );

    expect(screen.queryByTestId('audio-modal')).not.toBeInTheDocument();

    fireEvent.click(
      screen.getByRole('button', { name: '1.01a demo reading.mp3' }),
    );

    const player = screen.getByTestId('uploaded-audio-player');
    expect(player).toBeInTheDocument();
    expect(player.getAttribute('controls')).not.toBeNull();
    expect(player.getAttribute('src')).toContain(
      '/api/v1/agents/attachments/attach-1/preview',
    );
  });

  it('opens an inline player for an unsent local audio file', () => {
    const localFile = new File([new Uint8Array([1])], 'note.mp3', {
      type: 'audio/mpeg',
    });

    render(<UploadedMessageFiles files={[localFile]}></UploadedMessageFiles>);

    fireEvent.click(screen.getByRole('button', { name: 'note.mp3' }));

    const player = screen.getByTestId('uploaded-audio-player');
    expect(player.getAttribute('src')).toBe('blob:local-audio');
  });

  it('does not open a player for image attachments', () => {
    const image = {
      ...uploadedAudio,
      mime_type: 'image/png',
      name: 'photo.png',
    };

    render(<UploadedMessageFiles files={[image]}></UploadedMessageFiles>);

    fireEvent.click(screen.getByText('photo.png'));

    expect(screen.queryByTestId('audio-modal')).not.toBeInTheDocument();
  });

  it('keeps the spinner while the remote audio preview is loading', () => {
    mockUseAuthenticatedImageUrl.mockReturnValueOnce({
      src: '',
      status: 'loading',
    });

    render(
      <UploadedMessageFiles files={[uploadedAudio]}></UploadedMessageFiles>,
    );

    fireEvent.click(
      screen.getByRole('button', { name: '1.01a demo reading.mp3' }),
    );

    expect(screen.getByTestId('spin')).toBeInTheDocument();
    expect(
      screen.queryByTestId('uploaded-audio-player'),
    ).not.toBeInTheDocument();
  });

  it('shows an error state instead of a spinner when the remote audio fetch fails', () => {
    mockUseAuthenticatedImageUrl.mockReturnValueOnce({
      src: '',
      status: 'error',
    });

    render(
      <UploadedMessageFiles files={[uploadedAudio]}></UploadedMessageFiles>,
    );

    fireEvent.click(
      screen.getByRole('button', { name: '1.01a demo reading.mp3' }),
    );

    expect(screen.getByTestId('uploaded-audio-error')).toBeInTheDocument();
    expect(
      screen.queryByTestId('uploaded-audio-player'),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId('spin')).not.toBeInTheDocument();
  });
});
