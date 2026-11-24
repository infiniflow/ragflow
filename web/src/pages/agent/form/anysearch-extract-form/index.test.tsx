import { TooltipProvider } from '@/components/ui/tooltip';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { initialAnySearchExtractValues } from '../../constant';
import AnySearchExtractForm from './index';

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
// The real editor is a Lexical instance that needs the canvas store and the
// variable picker. A plain input keeps the test on the form's own wiring.
jest.mock('../components/prompt-editor', () => {
  const { forwardRef }: typeof import('react') = require('react');
  return {
    PromptEditor: forwardRef<
      HTMLInputElement,
      { value?: string; onChange?: (value?: string) => void }
    >(function PromptEditor({ value, onChange }, ref) {
      function handleChange(event: React.ChangeEvent<HTMLInputElement>) {
        onChange?.(event.target.value);
      }
      return (
        <input
          ref={ref}
          aria-label="URL"
          value={value ?? ''}
          onChange={handleChange}
        />
      );
    }),
  };
});
// The real hook writes to the canvas store. Capture the form instead so the
// test can read the values the node would save.
jest.mock('../../hooks/use-watch-form-change', () => ({
  useWatchFormChange: (_id: string | undefined, form: any) => {
    mockForm = form;
  },
}));

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const NodeSettingKeys = ['api_key', 'outputs', 'url'];

function renderNodeForm(form: Record<string, any> = {}) {
  const node = { id: 'AnySearchExtract:0', data: { form } };
  return render(
    <TooltipProvider>
      <AnySearchExtractForm node={node as any} />
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

describe('AnySearch Extract node form', () => {
  beforeAll(() => {
    (globalThis as Record<string, unknown>).ResizeObserver = ResizeObserverStub;
  });

  beforeEach(() => {
    mockForm = undefined;
  });

  it('renders the settings and the declared outputs', () => {
    renderNodeForm();

    expect(screen.getByLabelText('flow.apiKey')).toBeInTheDocument();
    expect(screen.getByLabelText('URL')).toHaveValue('');
    expect(screen.getByText('formalized_content:')).toBeInTheDocument();
    expect(screen.getByText('json:')).toBeInTheDocument();
  });

  it('keeps only the key, url and outputs as node settings', () => {
    renderNodeForm();

    expect(Object.keys(mockForm.getValues()).sort()).toEqual(NodeSettingKeys);
    expect(Object.keys(initialAnySearchExtractValues).sort()).toEqual(
      NodeSettingKeys,
    );
  });

  it('saves a blank key because AnySearch works anonymously', async () => {
    renderNodeForm({ api_key: '', url: 'https://example.com' });

    expect(screen.getByLabelText('flow.apiKey')).toHaveAttribute(
      'placeholder',
      'flow.anySearchApiKeyTip',
    );
    const save = await submit();

    expect(save).toHaveBeenCalledTimes(1);
    expect(save.mock.calls[0][0]).toMatchObject({
      api_key: '',
      url: 'https://example.com',
    });
  });

  it.each([
    'https://example.com/a?b=1',
    '{sys.query}',
    'ftp://not-http.example.com',
    'localhost',
  ])('saves the url %p without form-side URL validation', async (url) => {
    renderNodeForm();

    fireEvent.change(screen.getByLabelText('URL'), { target: { value: url } });
    const save = await submit();

    expect(save).toHaveBeenCalledTimes(1);
    expect(save.mock.calls[0][0]).toMatchObject({ url });
  });

  it('does not save a blank url, like Search1API crawl', async () => {
    renderNodeForm();

    const save = await submit();

    expect(save).not.toHaveBeenCalled();
    expect(mockForm.formState.errors.url).toBeDefined();
  });

  it('shows saved values again after a JSON save and reload', async () => {
    const { unmount } = renderNodeForm();
    fireEvent.change(screen.getByLabelText('flow.apiKey'), {
      target: { value: 'anysearch-test-key' },
    });
    fireEvent.change(screen.getByLabelText('URL'), {
      target: { value: 'https://example.com' },
    });
    const saved = JSON.parse(JSON.stringify(mockForm.getValues()));
    unmount();

    renderNodeForm(saved);

    expect(screen.getByLabelText('flow.apiKey')).toHaveValue(
      'anysearch-test-key',
    );
    expect(screen.getByLabelText('URL')).toHaveValue('https://example.com');
  });
});
