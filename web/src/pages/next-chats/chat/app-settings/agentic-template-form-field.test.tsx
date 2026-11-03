import { Form } from '@/components/ui/form';
import { TooltipProvider } from '@/components/ui/tooltip';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { AgenticTemplateFormField } from './agentic-template-form-field';

const DefaultTemplate = 'default template';
const FieldName = 'prompt_config.agentic_rag.content';
const Config = {
  defaults: { content: DefaultTemplate },
  override: null,
};

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
        <AgenticTemplateFormField name={FieldName} config={Config} />
      </Form>
    </TooltipProvider>
  );
}

describe('AgenticTemplateFormField', () => {
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

  it('resets custom content to the shipped template', () => {
    render(
      <Harness
        defaultValues={{
          prompt_config: { agentic_rag: { content: 'custom template' } },
        }}
      />,
    );

    fireEvent.click(
      screen.getByRole('button', { name: 'chat.resetToDefault' }),
    );

    expect(screen.getByRole('textbox')).toHaveValue(DefaultTemplate);
  });

  it('keeps the field empty after the autofilled template is cleared', async () => {
    render(<Harness defaultValues={{ prompt_config: {} }} />);
    const textarea = screen.getByRole('textbox');

    await waitFor(() => expect(textarea).toHaveValue(DefaultTemplate));
    fireEvent.change(textarea, { target: { value: '' } });

    await waitFor(() => expect(textarea).toHaveValue(''));
  });
});
