import { SharedFrom } from '@/constants/chat';
import { TooltipProvider } from '@/components/ui/tooltip';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/routes', () => ({
  Routes: {
    ChatWidget: '/chat/widget',
    AgentShare: '/agent/share',
    ChatShare: '/chat/share',
  },
}));

jest.mock('@/components/theme-provider', () => ({
  useIsDarkTheme: () => false,
}));

jest.mock('react-syntax-highlighter', () => ({
  Prism: ({ children }: { children?: React.ReactNode }) => (
    <pre>{children}</pre>
  ),
}));
jest.mock('react-syntax-highlighter/dist/esm/styles/prism', () => ({
  oneDark: {},
  oneLight: {},
}));

import EmbedDialog from '..';

const renderAgentEmbedDialog = (props: Record<string, unknown> = {}) =>
  render(
    <TooltipProvider>
      <EmbedDialog
        visible
        hideModal={jest.fn()}
        token="agent-token"
        from={SharedFrom.Agent}
        beta="beta-token"
        isAgent
        {...props}
      />
    </TooltipProvider>,
  );

beforeAll(() => {
  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  Element.prototype.scrollIntoView = () => {};
});

describe('EmbedDialog widget settings persistence', () => {
  it('saves the full dialog state including the Embed Setup fields', async () => {
    const onSaveWidgetSettings = jest.fn().mockResolvedValue(undefined);
    renderAgentEmbedDialog({ onSaveWidgetSettings });

    fireEvent.click(screen.getByRole('radio', { name: 'chat.dark' }));
    fireEvent.click(
      screen.getByRole('button', { name: 'flow.save widget settings' }),
    );

    await waitFor(() => expect(onSaveWidgetSettings).toHaveBeenCalledTimes(1));
    expect(onSaveWidgetSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        theme: 'dark',
        embedType: 'fullscreen',
        visibleAvatar: false,
        published: false,
        locale: '',
        userId: '',
        widgetTitle: '',
        widgetAccentColor: '#2563eb',
      }),
    );
  });

  it('restores the saved theme when reopened', () => {
    renderAgentEmbedDialog({ initialWidgetSettings: { theme: 'dark' } });

    expect(screen.getByRole('radio', { name: 'chat.dark' })).toBeChecked();
    expect(screen.getByRole('radio', { name: 'chat.light' })).not.toBeChecked();
  });
});
