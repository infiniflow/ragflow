import { Form } from '@/components/ui/form';
import { fireEvent, render, screen } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { TableColumnSettingsFields } from '../table-column-settings-form-fields';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

type Props = {
  mode: unknown;
  columns: unknown;
  roles: Record<string, string | undefined> | null;
  onModeChange: (mode: 'auto' | 'manual') => void;
  onRoleChange: (column: string, role: string) => void;
};

function Harness(props: Props) {
  const form = useForm<{ value: string }>({ defaultValues: { value: '' } });
  return (
    <Form {...form}>
      <TableColumnSettingsFields idPrefix="harness" {...props} />
    </Form>
  );
}

describe('TableColumnSettingsFields', () => {
  it('offers no role picker until a mode is stated', () => {
    render(
      <Harness
        mode={undefined}
        columns={[]}
        roles={{}}
        onModeChange={jest.fn()}
        onRoleChange={jest.fn()}
      />,
    );

    expect(
      screen.getByText('tableColumnModeAutoDescription'),
    ).toBeInTheDocument();
    expect(screen.queryAllByRole('combobox')).toHaveLength(0);
  });

  // A saved profile that never chose one reads as auto, exactly as the runtime
  // reads it: only the exact "manual" selects manual.
  it('renders a mode outside the vocabulary as auto', () => {
    render(
      <Harness
        mode="Manual"
        columns={['a']}
        roles={{}}
        onModeChange={jest.fn()}
        onRoleChange={jest.fn()}
      />,
    );

    expect(
      screen.getByText('tableColumnModeAutoDescription'),
    ).toBeInTheDocument();
    expect(screen.queryByText('tableColumnRolesTip')).toBeNull();
  });

  it('names one column row per discovered column under manual', () => {
    render(
      <Harness
        mode="manual"
        columns={['region', 'revenue']}
        roles={{ region: 'indexing' }}
        onModeChange={jest.fn()}
        onRoleChange={jest.fn()}
      />,
    );

    expect(screen.getByText('tableColumnRolesTip')).toBeInTheDocument();
    expect(screen.getByText('region')).toBeInTheDocument();
    expect(screen.getByText('revenue')).toBeInTheDocument();
    expect(screen.getAllByRole('combobox')).toHaveLength(2);
  });

  it('asks for a configuration when manual has no columns to configure', () => {
    render(
      <Harness
        mode="manual"
        columns={undefined}
        roles={{}}
        onModeChange={jest.fn()}
        onRoleChange={jest.fn()}
      />,
    );

    expect(screen.getByText('tableColumnRolesEmpty')).toBeInTheDocument();
  });

  it('reports the mode the reader picked', () => {
    const onModeChange = jest.fn();
    render(
      <Harness
        mode="auto"
        columns={['a']}
        roles={{}}
        onModeChange={onModeChange}
        onRoleChange={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByLabelText('tableColumnModeManual'));
    expect(onModeChange).toHaveBeenCalledWith('manual');
  });
});
