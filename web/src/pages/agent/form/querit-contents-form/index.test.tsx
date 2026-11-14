import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { TooltipProvider } from '@/components/ui/tooltip';
import useGraphStore from '../../store';
import QueritContentsForm from '.';

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));

jest.mock('@/components/ui/tooltip', () => ({
  TooltipProvider: ({ children }: { children: React.ReactNode }) => children,
  FormTooltip: () => null,
}));

jest.mock('@/components/originui/select-with-search', () => ({
  SelectWithSearch: () => null,
}));

jest.mock('@/components/ui/switch', () => ({
  Switch: () => null,
}));

jest.mock('../components/api-key-field', () => ({
  ApiKeyField: () => null,
}));

jest.mock('../components/form-wrapper', () => ({
  FormWrapper: ({ children }: { children: React.ReactNode }) => children,
}));

jest.mock('../components/output', () => ({
  Output: () => null,
}));

jest.mock('../components/prompt-editor', () => ({
  PromptEditor: () => null,
}));

describe('QueritContentsForm', () => {
  afterEach(() => {
    useGraphStore.setState({ nodes: [] });
  });

  it('keeps the last valid timeout while the field is cleared and can be replaced', async () => {
    const node = {
      id: 'querit-node',
      data: {
        form: {
          api_key: '',
          urls: 'https://example.com',
          format: 'markdown',
          crawl_timeout: 10,
          extras_meta: false,
        },
      },
    } as any;
    useGraphStore.setState({ nodes: [node] });

    render(
      <TooltipProvider>
        <QueritContentsForm node={node} />
      </TooltipProvider>,
    );
    const timeoutInput = screen.getByRole('spinbutton');
    fireEvent.change(timeoutInput, {
      target: { value: '' },
    });

    await waitFor(() => {
      expect(useGraphStore.getState().nodes[0].data.form.crawl_timeout).toBe(
        10,
      );
    });
    expect((timeoutInput as HTMLInputElement).value).toBe('');

    fireEvent.change(timeoutInput, { target: { value: '25' } });

    await waitFor(() => {
      expect(useGraphStore.getState().nodes[0].data.form.crawl_timeout).toBe(
        25,
      );
    });
    expect(timeoutInput).toHaveValue(25);
  });

  it('restores the last valid timeout if the cleared field is left empty', async () => {
    const node = {
      id: 'querit-node',
      data: {
        form: {
          api_key: '',
          urls: 'https://example.com',
          format: 'markdown',
          crawl_timeout: 25,
          extras_meta: false,
        },
      },
    } as any;
    useGraphStore.setState({ nodes: [node] });

    render(
      <TooltipProvider>
        <QueritContentsForm node={node} />
      </TooltipProvider>,
    );
    const timeoutInput = screen.getByRole('spinbutton');
    fireEvent.change(timeoutInput, { target: { value: '' } });
    fireEvent.blur(timeoutInput);

    await waitFor(() => {
      expect(useGraphStore.getState().nodes[0].data.form.crawl_timeout).toBe(
        25,
      );
    });
    expect(timeoutInput).toHaveValue(25);
  });

  it('normalizes a previously saved empty crawl timeout to its default', async () => {
    const node = {
      id: 'querit-node',
      data: {
        form: {
          api_key: '',
          urls: 'https://example.com',
          format: 'markdown',
          crawl_timeout: '',
          extras_meta: false,
        },
      },
    } as any;
    useGraphStore.setState({ nodes: [node] });

    render(
      <TooltipProvider>
        <QueritContentsForm node={node} />
      </TooltipProvider>,
    );

    await waitFor(() => {
      expect(useGraphStore.getState().nodes[0].data.form.crawl_timeout).toBe(
        10,
      );
    });
    expect(screen.getByRole('spinbutton')).toHaveValue(10);
  });
});
