import { Form } from '@/components/ui/form';
import { TooltipProvider } from '@/components/ui/tooltip';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { AgenticTemplateFormField } from './agentic-template-form-field';

const DefaultTemplate = 'default template';
const FieldName = 'prompt_config.agentic_rag.content';

const mockUseFetchAgenticRagConfig = jest.fn();

jest.mock('@/hooks/use-chat-request', () => ({
  useFetchAgenticRagConfig: (...args: unknown[]) =>
    mockUseFetchAgenticRagConfig(...args),
}));

jest.mock('react-router', () => ({
  useParams: () => ({ id: 'chat-1' }),
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

function Harness({
  defaultValues,
}: {
  defaultValues: Record<string, unknown>;
}) {
  const form = useForm({ defaultValues, mode: 'onChange' });

  return (
    <TooltipProvider>
      <Form {...form}>
        <AgenticTemplateFormField name={FieldName} />
      </Form>
    </TooltipProvider>
  );
}

describe('AgenticTemplateFormField', () => {
  beforeEach(() => {
    mockUseFetchAgenticRagConfig.mockReturnValue({
      data: {
        defaults: { content: DefaultTemplate },
        override: null,
      },
    });
  });

  it('fills the shipped template only when the field has no value', async () => {
    render(<Harness defaultValues={{ prompt_config: {} }} />);

    await waitFor(() =>
      expect(screen.getByRole('textbox')).toHaveValue(DefaultTemplate),
    );
  });

  it('keeps an explicitly blank value instead of autofilling it', () => {
    render(
      <Harness
        defaultValues={{
          prompt_config: { agentic_rag: { content: '' } },
        }}
      />,
    );

    expect(screen.getByRole('textbox')).toHaveValue('');
  });

  it('keeps the field empty after the autofilled template is cleared', async () => {
    render(<Harness defaultValues={{ prompt_config: {} }} />);
    const textarea = screen.getByRole('textbox');

    await waitFor(() => expect(textarea).toHaveValue(DefaultTemplate));
    fireEvent.change(textarea, { target: { value: '' } });

    await waitFor(() => expect(textarea).toHaveValue(''));
  });
});
