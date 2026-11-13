import { fireEvent, render, screen } from '@testing-library/react';
import { NextMessageInput } from './next';

// This file uses jest.mock: keep imported bindings out of type positions because
// the esbuild-jest babel pass does not strip TypeScript from mock factories.
const mockOnPressEnter = jest.fn();
const mockOnInputChange = jest.fn();
const mockSetThinkingLevel = jest.fn();
let mockThinkingLevel = '0';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/use-thinking-level', () => ({
  ThinkingMode: { Naive: '0', Agentic: '5' },
  setThinkingLevel: (value: string) => mockSetThinkingLevel(value),
  useThinkingLevel: () => mockThinkingLevel,
}));

jest.mock('../ui/audio-button', () => ({
  AudioButton: () => <button data-testid="chat-detail-audio-toggle" />,
}));

type InputOverrides = {
  disabled?: boolean;
  value?: string;
  sendDisabled?: boolean;
  sendLoading?: boolean;
  isUploading?: boolean;
  showUploadIcon?: boolean;
  showReasoning?: boolean;
  showInternet?: boolean;
};

function renderInput(overrides: InputOverrides = {}) {
  return render(
    <NextMessageInput
      disabled={false}
      value="Hello"
      sendDisabled={false}
      sendLoading={false}
      conversationId="conversation-1"
      onPressEnter={mockOnPressEnter}
      onInputChange={mockOnInputChange}
      {...overrides}
    />,
  );
}

function openAddMenu() {
  // Radix menus open reliably in jsdom through their keyboard trigger.
  fireEvent.keyDown(screen.getByTestId('chat-detail-add-menu'), {
    key: 'ArrowDown',
  });
}

beforeAll(() => {
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  if (!Element.prototype.hasPointerCapture) {
    Element.prototype.hasPointerCapture = () => false;
  }
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {};
  }
});

beforeEach(() => {
  mockOnPressEnter.mockReset();
  mockOnInputChange.mockReset();
  mockSetThinkingLevel.mockReset();
  mockThinkingLevel = '0';
});

afterEach(() => {
  jest.restoreAllMocks();
});

describe('NextMessageInput', () => {
  it('shows the upload and conditionally enabled web search actions', async () => {
    renderInput({ showInternet: true });
    openAddMenu();

    expect(await screen.findByTestId('chat-detail-attach')).toHaveTextContent(
      'chat.addPhotosAndFiles',
    );
    expect(screen.getByTestId('chat-detail-internet-toggle')).toHaveTextContent(
      'chat.webSearch',
    );
    expect(screen.getByTestId('chat-detail-internet-toggle')).toHaveAttribute(
      'role',
      'menuitem',
    );
    expect(
      screen.getByTestId('chat-detail-internet-toggle'),
    ).not.toHaveAttribute('aria-checked');
  });

  it('keeps web search available when file upload is hidden', async () => {
    renderInput({ showUploadIcon: false, showInternet: true });
    openAddMenu();

    expect(
      await screen.findByTestId('chat-detail-internet-toggle'),
    ).toBeInTheDocument();
    expect(screen.queryByTestId('chat-detail-attach')).not.toBeInTheDocument();
  });

  it('hides web search without changing upload visibility', async () => {
    renderInput({ showInternet: false });
    openAddMenu();

    expect(await screen.findByTestId('chat-detail-attach')).toBeInTheDocument();
    expect(
      screen.queryByTestId('chat-detail-internet-toggle'),
    ).not.toBeInTheDocument();
  });

  it('shows the web search indicator only while selected', async () => {
    renderInput({ showInternet: true });
    expect(
      screen.queryByTestId('chat-detail-internet-active'),
    ).not.toBeInTheDocument();

    openAddMenu();
    fireEvent.click(await screen.findByTestId('chat-detail-internet-toggle'));

    const indicator = await screen.findByTestId('chat-detail-internet-active');
    expect(indicator).toHaveTextContent('chat.webSearch');
    expect(indicator).toHaveClass('text-accent-primary');

    fireEvent.click(screen.getByTestId('chat-detail-internet-remove'));
    expect(
      screen.queryByTestId('chat-detail-internet-active'),
    ).not.toBeInTheDocument();
  });

  it('opens file selection from the upload menu item', async () => {
    const inputClick = jest
      .spyOn(HTMLInputElement.prototype, 'click')
      .mockImplementation(() => {});
    renderInput();
    openAddMenu();

    fireEvent.click(await screen.findByTestId('chat-detail-attach'));

    expect(inputClick).toHaveBeenCalledTimes(1);
  });

  it('submits the selected web search state with the message', async () => {
    renderInput({ showInternet: true });
    openAddMenu();
    fireEvent.click(await screen.findByTestId('chat-detail-internet-toggle'));

    expect(
      await screen.findByTestId('chat-detail-internet-active'),
    ).toBeInTheDocument();

    const sendButton = screen.getByTestId('chat-detail-send');
    expect(sendButton.querySelector('.lucide-arrow-up')).toBeInTheDocument();
    fireEvent.click(sendButton);

    expect(mockOnPressEnter).toHaveBeenCalledWith({
      enableThinking: '0',
      enableInternet: true,
    });
  });

  it('keeps the existing thinking options and selection behavior', async () => {
    renderInput({ showReasoning: true });

    fireEvent.click(screen.getByTestId('chat-detail-thinking-toggle'));
    expect(
      await screen.findByText('chat.thinkingLevelAgenticDescription'),
    ).toBeInTheDocument();
    expect(screen.getAllByText('chat.thinkingLevelNone')).not.toHaveLength(0);
    fireEvent.click(screen.getByText('chat.thinkingLevelAgentic'));

    expect(mockSetThinkingLevel).toHaveBeenCalledWith('5');
  });
});
