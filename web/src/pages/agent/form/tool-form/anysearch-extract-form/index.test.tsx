import { TooltipProvider } from '@/components/ui/tooltip';
import { act, fireEvent, render, screen } from '@testing-library/react';
import AnySearchExtractToolForm from './index';

let mockValues: Record<string, any> = {};
let mockForm: any;

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));
jest.mock('i18next', () => ({
  ...jest.requireActual('i18next'),
  t: (key: string) => key,
}));
jest.mock('react-i18next', () => ({
  ...jest.requireActual('react-i18next'),
  useTranslation: () => ({ t: (key: string) => key }),
}));
// The standalone node form shares this module and pulls in the Lexical
// editor. The tool form never renders the URL field.
jest.mock('../../components/prompt-editor', () => ({
  PromptEditor: () => null,
}));
jest.mock('../use-values', () => ({ useValues: () => mockValues }));
jest.mock('../use-watch-change', () => ({
  useWatchFormChange: (form: any) => {
    mockForm = form;
  },
}));

function renderToolForm(values: Record<string, any>) {
  mockValues = values;
  return render(
    <TooltipProvider>
      <AnySearchExtractToolForm />
    </TooltipProvider>,
  );
}

async function submit() {
  const save = jest.fn();
  await act(async () => {
    await mockForm!.handleSubmit(save)();
  });
  return save;
}

describe('AnySearch Extract tool form', () => {
  beforeEach(() => {
    mockForm = undefined;
  });

  it('saves a blank key because AnySearch works anonymously', async () => {
    renderToolForm({ api_key: '' });

    expect(screen.getByLabelText('flow.apiKey')).toHaveAttribute(
      'placeholder',
      'flow.anySearchApiKeyTip',
    );
    const save = await submit();

    expect(save).toHaveBeenCalledTimes(1);
    expect(save.mock.calls[0][0]).toEqual({ api_key: '' });
  });

  it('offers no URL field because the agent supplies the URL', () => {
    renderToolForm({ api_key: '' });

    expect(screen.queryByText('flow.anySearchExtractUrl')).toBeNull();
  });

  it('shows a saved key again after a JSON save and reload', async () => {
    const { unmount } = renderToolForm({ api_key: '' });
    fireEvent.change(screen.getByLabelText('flow.apiKey'), {
      target: { value: 'anysearch-test-key' },
    });
    const saved = JSON.parse(JSON.stringify((await submit()).mock.calls[0][0]));
    unmount();

    renderToolForm(saved);

    expect(saved).toEqual({ api_key: 'anysearch-test-key' });
    expect(screen.getByLabelText('flow.apiKey')).toHaveValue(
      'anysearch-test-key',
    );
  });
});
