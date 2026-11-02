import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';
import { WebSearchProvider } from '@/constants/chat';
import { TooltipProvider } from '@/components/ui/tooltip';
import { hasWebSearchProvider } from '@/pages/next-chats/chat/web-search-api-key';
import { WebSearchFormField } from './web-search-form-field';

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({
    t: (key: string, values?: { provider?: string }) =>
      key === 'webSearchApiKeyLabel' ? `${values?.provider} API key` : key,
  }),
}));
// Exercise catalog values, React Hook Form state and the real key field without
// coupling this provider regression to the searchable dropdown's popover.
jest.mock('./originui/select-with-search', () => ({
  SelectWithSearch: ({
    value,
    onChange,
    options,
  }: {
    value?: string;
    onChange: (value: string) => void;
    options: { value: string }[];
  }) => {
    function handleChange(event: React.ChangeEvent<HTMLSelectElement>) {
      onChange(event.target.value);
    }
    return (
      <select aria-label="Provider" value={value ?? ''} onChange={handleChange}>
        <option value="">Clear</option>
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.value}
          </option>
        ))}
      </select>
    );
  },
}));

describe('AnySearch settings field', () => {
  it('selects, saves, reloads and clears AnySearch with an optional key', async () => {
    const save = jest.fn();
    function Settings({ saved = {} }: { saved?: Record<string, string> }) {
      const form = useForm({ defaultValues: { prompt_config: saved } });
      return (
        <FormProvider {...form}>
          <form onSubmit={form.handleSubmit(save)}>
            <TooltipProvider>
              <WebSearchFormField />
            </TooltipProvider>
            <button type="submit">Save</button>
          </form>
        </FormProvider>
      );
    }
    const { unmount } = render(<Settings />);
    const select = screen.getByRole('combobox', { name: 'Provider' });
    expect(select.querySelectorAll('option')[1]).toHaveValue(
      WebSearchProvider.AnySearch,
    );
    fireEvent.change(select, {
      target: { value: WebSearchProvider.AnySearch },
    });
    const key = screen.getByLabelText('AnySearch API key');
    expect(key).not.toBeRequired();
    expect(key).toHaveAttribute('placeholder', 'anysearchApiKeyMessage');
    expect(
      screen.getByRole('link', { name: 'webSearchApiKeyHelp' }),
    ).toHaveAttribute('href', 'https://anysearch.com/console/api-keys');
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    const saved = JSON.parse(
      JSON.stringify(save.mock.calls[0][0].prompt_config),
    );
    expect(hasWebSearchProvider(saved)).toBe(true);
    unmount();

    render(<Settings saved={saved} />);
    expect(screen.getByRole('combobox', { name: 'Provider' })).toHaveValue(
      WebSearchProvider.AnySearch,
    );
    fireEvent.change(screen.getByLabelText('AnySearch API key'), {
      target: { value: 'anysearch-test' },
    });
    fireEvent.change(screen.getByRole('combobox', { name: 'Provider' }), {
      target: { value: '' },
    });
    expect(
      screen.queryByLabelText('AnySearch API key'),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(save).toHaveBeenCalledTimes(2));
    const cleared = JSON.parse(
      JSON.stringify(save.mock.calls[1][0].prompt_config),
    );
    expect(cleared.anysearch_api_key).toBe('anysearch-test');
    expect(hasWebSearchProvider(cleared)).toBe(false);
  });
});
