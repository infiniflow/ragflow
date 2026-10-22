import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { FormProvider, useForm, useWatch } from 'react-hook-form';
import { TooltipProvider } from '@/components/ui/tooltip';
import { HyperlinkDefaultSetup } from '../../constant/pipeline';
import { HyperlinkFormFields } from './hyperlink-form-fields';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

function HyperlinkForm({ showMarkdownLinks = false, fetchLinks = false }) {
  const form = useForm({
    defaultValues: {
      setups: [{ ...HyperlinkDefaultSetup, fetch_links: fetchLinks }],
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

const savedSetup = () =>
  JSON.parse(screen.getByTestId('saved-setup').textContent!);

describe('hyperlink parser fields', () => {
  it('hides the markdown-only switch outside the markdown family', async () => {
    // The gated block has to be open for this to say anything about the
    // markdown switch: with fetching off, everything is hidden anyway.
    render(<HyperlinkForm fetchLinks />);

    await waitFor(() => {
      expect(screen.getByText('flow.hyperlinkMaxUrls')).toBeTruthy();
    });
    expect(screen.queryByText('flow.collectMarkdownLinks')).toBeNull();
  });

  it('shows the markdown-only switch for the markdown family', async () => {
    render(<HyperlinkForm fetchLinks showMarkdownLinks />);

    await waitFor(() => {
      expect(screen.getByText('flow.collectMarkdownLinks')).toBeTruthy();
    });
  });

  it('shows the budgets only after fetching is enabled', async () => {
    render(<HyperlinkForm showMarkdownLinks />);

    expect(screen.queryByText('flow.hyperlinkMaxUrls')).toBeNull();

    fireEvent.click(screen.getByRole('switch'));

    await waitFor(() => {
      expect(screen.getByText('flow.hyperlinkMaxUrls')).toBeTruthy();
    });
  });

  it('saves both switches and a typed budget', async () => {
    render(<HyperlinkForm fetchLinks showMarkdownLinks />);

    await waitFor(() => {
      expect(screen.getByText('flow.collectMarkdownLinks')).toBeTruthy();
    });
    fireEvent.click(screen.getAllByRole('switch')[1]);
    // max_urls is the first budget input; all four are number inputs.
    fireEvent.change(screen.getAllByRole('spinbutton')[0], {
      target: { value: '9' },
    });

    await waitFor(() => {
      expect(savedSetup()).toEqual(
        expect.objectContaining({
          fetch_links: true,
          collect_markdown_links: true,
          max_urls: 9,
        }),
      );
    });
  });
});
