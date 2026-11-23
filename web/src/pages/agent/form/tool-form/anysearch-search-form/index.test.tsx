import { TooltipProvider } from '@/components/ui/tooltip';
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import AnySearchSearchToolForm from './index';

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
// The standalone node form shares this module and pulls in the app router,
// which needs browser globals jsdom lacks. The tool form never renders it.
jest.mock('../../components/query-variable', () => ({
  QueryVariable: () => null,
}));
jest.mock('../use-values', () => ({ useValues: () => mockValues }));
jest.mock('../use-watch-change', () => ({
  useWatchFormChange: (form: any) => {
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

const defaultValues = { api_key: '', top_n: 10, zone: '', language: '' };

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

function renderToolForm(values: Record<string, any>) {
  mockValues = values;
  return render(
    <TooltipProvider>
      <AnySearchSearchToolForm />
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

describe('AnySearch Search tool form', () => {
  beforeAll(() => {
    (globalThis as Record<string, unknown>).ResizeObserver = ResizeObserverStub;
  });

  beforeEach(() => {
    mockForm = undefined;
  });

  it('saves a blank key because AnySearch works anonymously', async () => {
    renderToolForm(defaultValues);

    expect(screen.getByLabelText('flow.apiKey')).toHaveAttribute(
      'placeholder',
      'flow.anySearchApiKeyTip',
    );
    const save = await submit();

    expect(save).toHaveBeenCalledTimes(1);
    expect(save.mock.calls[0][0]).toEqual(defaultValues);
  });

  it('saves a typed key, zone and language', async () => {
    renderToolForm(defaultValues);

    fireEvent.change(screen.getByLabelText('flow.apiKey'), {
      target: { value: 'anysearch-test-key' },
    });
    fireEvent.change(screen.getByRole('combobox', { name: 'Zone' }), {
      target: { value: 'cn' },
    });
    fireEvent.change(screen.getByLabelText('flow.anySearchLanguage'), {
      target: { value: 'zh-CN' },
    });
    const save = await submit();

    expect(save.mock.calls[0][0]).toEqual({
      api_key: 'anysearch-test-key',
      top_n: 10,
      zone: 'cn',
      language: 'zh-CN',
    });
  });

  it('offers only the intl and cn zones and allows unsetting one', async () => {
    renderToolForm({ ...defaultValues, zone: 'intl' });

    const zone = screen.getByRole('combobox', { name: 'Zone' });
    expect(
      Array.from(zone.querySelectorAll('option')).map((x) => x.value),
    ).toEqual(['', 'intl', 'cn']);
    expect(zone).toHaveValue('intl');
    fireEvent.change(zone, { target: { value: '' } });
    const save = await submit();

    expect(save.mock.calls[0][0].zone).toBe('');
  });

  it('keeps Top N within 1 and 10 when typed', async () => {
    renderToolForm(defaultValues);
    const topN = screen.getByRole('spinbutton');

    fireEvent.change(topN, { target: { value: '11' } });
    fireEvent.blur(topN);
    expect(topN).toHaveValue(10);
    expect((await submit()).mock.calls[0][0].top_n).toBe(10);

    fireEvent.change(topN, { target: { value: '0' } });
    fireEvent.blur(topN);
    expect(topN).toHaveValue(1);
    expect((await submit()).mock.calls[0][0].top_n).toBe(1);
  });

  it.each([0, 11, 2.5])('rejects the out of range Top N %p', async (topN) => {
    renderToolForm(defaultValues);

    act(() => {
      mockForm!.setValue('top_n', topN);
    });
    const save = await submit();

    expect(save).not.toHaveBeenCalled();
    await waitFor(() => expect(mockForm!.formState.errors.top_n).toBeDefined());
  });

  it('shows saved values again after a JSON save and reload', async () => {
    const { unmount } = renderToolForm(defaultValues);
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
    const saved = JSON.parse(JSON.stringify((await submit()).mock.calls[0][0]));
    unmount();

    renderToolForm(saved);

    expect(saved.top_n).toBe(4);
    expect(screen.getByLabelText('flow.apiKey')).toHaveValue(
      'anysearch-test-key',
    );
    expect(screen.getByRole('combobox', { name: 'Zone' })).toHaveValue('intl');
    expect(screen.getByLabelText('flow.anySearchLanguage')).toHaveValue('en');
    expect(screen.getByRole('spinbutton')).toHaveValue(4);
  });
});
