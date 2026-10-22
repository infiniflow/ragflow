import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { TooltipProvider } from '@/components/ui/tooltip';
import { FormProvider, useForm, useWatch } from 'react-hook-form';
import { HyperlinkFormFields } from './hyperlink-form-fields';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

function HyperlinkForm({ showMarkdownLinks = false }) {
  const form = useForm({
    defaultValues: {
      setups: [
        {
          fetch_links: false,
          max_urls: 5,
          max_bytes_per_url: 1048576,
          max_total_bytes: 5242880,
          timeout_s: 10,
        },
      ],
    },
  });
  const setups = useWatch({ control: form.control, name: 'setups' });

  return (
    <TooltipProvider>
      <FormProvider {...form}>
        <HyperlinkFormFields
          prefix="setups.0"
          showMarkdownLinks={showMarkdownLinks}
        />
        <output data-testid="saved-setup">{JSON.stringify(setups[0])}</output>
      </FormProvider>
    </TooltipProvider>
  );
}

describe('hyperlink parser fields', () => {
  it('hides the markdown-only switch outside markdown', () => {
    render(<HyperlinkForm />);

    expect(screen.queryByText('flow.collectMarkdownLinks')).toBeNull();
  });

  it('shows the markdown switch and budgets only after fetching is enabled', async () => {
    render(<HyperlinkForm showMarkdownLinks />);

    expect(screen.queryByText('flow.collectMarkdownLinks')).toBeNull();
    expect(screen.queryByText('flow.hyperlinkMaxUrls')).toBeNull();

    fireEvent.click(screen.getByRole('switch'));

    await waitFor(() => {
      expect(screen.getByText('flow.collectMarkdownLinks')).toBeTruthy();
      expect(screen.getByText('flow.hyperlinkMaxUrls')).toBeTruthy();
    });

    fireEvent.click(screen.getAllByRole('switch')[1]);

    await waitFor(() => {
      expect(
        JSON.parse(screen.getByTestId('saved-setup').textContent!),
      ).toEqual(
        expect.objectContaining({
          fetch_links: true,
          collect_markdown_links: true,
        }),
      );
    });
  });
});
