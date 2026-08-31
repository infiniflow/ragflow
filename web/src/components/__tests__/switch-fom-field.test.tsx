import { Form } from '@/components/ui/form';
import { TooltipProvider } from '@/components/ui/tooltip';
import { fireEvent, render, screen } from '@testing-library/react';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { SwitchFormField } from '../switch-fom-field';

function Harness() {
  const form = useForm({ defaultValues: { flag: false } });
  return (
    <TooltipProvider>
      <Form {...form}>
        <SwitchFormField
          name="flag"
          label="Enable feature"
          tooltip="Explains the feature"
        />
      </Form>
    </TooltipProvider>
  );
}

function ErrorHarness() {
  const form = useForm({ defaultValues: { flag: false } });
  useEffect(() => {
    form.setError('flag', { type: 'required', message: 'Flag is required' });
  }, [form]);
  return (
    <TooltipProvider>
      <Form {...form}>
        <SwitchFormField name="flag" label="Enable feature" />
      </Form>
    </TooltipProvider>
  );
}

describe('SwitchFormField', () => {
  it('toggles the switch when the label text is clicked', () => {
    render(<Harness />);
    const switchEl = screen.getByRole('switch');
    expect(switchEl).toHaveAttribute('data-state', 'unchecked');

    fireEvent.click(screen.getByText('Enable feature'));

    expect(switchEl).toHaveAttribute('data-state', 'checked');
  });

  it('does not toggle the switch when the tooltip icon is clicked', () => {
    const { container } = render(<Harness />);
    const switchEl = screen.getByRole('switch');
    expect(switchEl).toHaveAttribute('data-state', 'unchecked');

    const icon = container.querySelector('svg.lucide');
    expect(icon).not.toBeNull();
    fireEvent.click(icon!);

    expect(switchEl).toHaveAttribute('data-state', 'unchecked');
  });

  it('renders the field error message', () => {
    render(<ErrorHarness />);

    expect(screen.getByText('Flag is required')).toBeInTheDocument();
  });
});
