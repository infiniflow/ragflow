import { TooltipProvider } from '@/components/ui/tooltip';
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import { initialAnySearchSearchValues } from '../../constant';
import AnySearchSearchForm from './index';

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
// The real selector pulls in the app router, which needs browser globals
// jsdom lacks (`Request is not defined`). Query binding is not under test.
jest.mock('../components/query-variable', () => ({
  QueryVariable: () => null,
}));
// The real hook writes to the canvas store. Capture the form instead so the
// test can read the values the node would save.
jest.mock('../../hooks/use-watch-form-change', () => ({
  useWatchFormChange: (_id: string | undefined, form: any) => {
    mockForm = form;
  },
}));
// Exercise the catalog values and React Hook Form state without coupling this
// test to the searchable dropdown's popover.
jest.mock('@/components/originui/select-with-search', () => {
  const { forwardRef }: typeof import('react') = require('react');
  return {
    SelectWithSearch: forwardRef<
      HTMLSelectElement,
      {
        value?: string;
        onChange: (value: string) => void;
        options: { value: string }[];
      }
    >(function SelectWithSearch({ value, onChange, options }, ref) {
      function handleChange(event: React.ChangeEvent<HTMLSelectElement>) {
        onChange(event.target.value);
      }
      return (
        <select
          ref={ref}
          aria-label="Zone"
          value={value ?? ''}
          onChange={handleChange}
        >
          <option value="">Clear</option>
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.value}
            </option>
          ))}
        </select>
      );
    }),
  };
});

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const NodeSettingKeys = [
  'api_key',
  'language',
  'outputs',
  'query',
  'top_n',
  'zone',
];

function renderNodeForm(form: Record<string, any> = {}) {
  const node = { id: 'AnySearchSearch:0', data: { form } };
  return render(
    <TooltipProvider>
      <AnySearchSearchForm node={node as any} />
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

describe('AnySearch Search node form', () => {
  beforeAll(() => {
    (globalThis as Record<string, unknown>).ResizeObserver = ResizeObserverStub;
  });

  beforeEach(() => {
    mockForm = undefined;
  });

  it('renders the settings and the declared outputs', () => {
    renderNodeForm();

    expect(screen.getByLabelText('flow.apiKey')).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Zone' })).toBeInTheDocument();
    expect(screen.getByLabelText('flow.anySearchLanguage')).toHaveValue('');
    expect(screen.getByRole('spinbutton')).toHaveValue(10);
    expect(screen.getByText('formalized_content:')).toBeInTheDocument();
    expect(screen.getByText('json:')).toBeInTheDocument();
  });

  it('saves a blank key because AnySearch works anonymously', async () => {
    renderNodeForm();

    expect(screen.getByLabelText('flow.apiKey')).toHaveAttribute(
      'placeholder',
      'flow.anySearchApiKeyTip',
    );
    const save = await submit();

    expect(save).toHaveBeenCalledTimes(1);
    expect(save.mock.calls[0][0]).toMatchObject({ api_key: '', top_n: 10 });
  });

  it('saves a typed key, zone, language and Top N', async () => {
    renderNodeForm();

    fireEvent.change(screen.getByLabelText('flow.apiKey'), {
      target: { value: 'anysearch-test-key' },
    });
    fireEvent.change(screen.getByRole('combobox', { name: 'Zone' }), {
      target: { value: 'cn' },
    });
    fireEvent.change(screen.getByLabelText('flow.anySearchLanguage'), {
      target: { value: 'zh-CN' },
    });
    fireEvent.change(screen.getByRole('spinbutton'), {
      target: { value: '4' },
    });
    const save = await submit();

    expect(save.mock.calls[0][0]).toMatchObject({
      api_key: 'anysearch-test-key',
      zone: 'cn',
      language: 'zh-CN',
      top_n: 4,
    });
  });

  it('shows saved values again after a JSON save and reload', async () => {
    const { unmount } = renderNodeForm();
    fireEvent.change(screen.getByLabelText('flow.apiKey'), {
      target: { value: 'anysearch-test-key' },
    });
    fireEvent.change(screen.getByRole('combobox', { name: 'Zone' }), {
      target: { value: 'intl' },
    });
    fireEvent.change(screen.getByLabelText('flow.anySearchLanguage'), {
      target: { value: 'en' },
    });
    fireEvent.change(screen.getByRole('spinbutton'), {
      target: { value: '4' },
    });
    const saved = JSON.parse(JSON.stringify(mockForm.getValues()));
    unmount();

    renderNodeForm(saved);

    expect(screen.getByLabelText('flow.apiKey')).toHaveValue(
      'anysearch-test-key',
    );
    expect(screen.getByRole('combobox', { name: 'Zone' })).toHaveValue('intl');
    expect(screen.getByLabelText('flow.anySearchLanguage')).toHaveValue('en');
    expect(screen.getByRole('spinbutton')).toHaveValue(4);
  });

  it.each([0, 11, 2.5])('rejects the out of range Top N %p', async (topN) => {
    renderNodeForm();

    act(() => {
      mockForm!.setValue('top_n', topN);
    });
    const save = await submit();

    expect(save).not.toHaveBeenCalled();
    await waitFor(() => expect(mockForm!.formState.errors.top_n).toBeDefined());
  });

  it('keeps tag and params out of the saved node settings', async () => {
    renderNodeForm();
    fireEvent.change(screen.getByLabelText('flow.apiKey'), {
      target: { value: 'anysearch-test-key' },
    });
    const save = await submit();

    expect(Object.keys(mockForm.getValues()).sort()).toEqual(NodeSettingKeys);
    expect(Object.keys(initialAnySearchSearchValues).sort()).toEqual(
      NodeSettingKeys,
    );
    const saved = save.mock.calls[0][0];
    expect(saved).not.toHaveProperty('tag');
    expect(saved).not.toHaveProperty('params');
  });
});
