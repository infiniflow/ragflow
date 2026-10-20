import { fireEvent, render, screen } from '@testing-library/react';
import { useForm } from 'react-hook-form';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { LayoutRecognizeFormField } from './layout-recognize-form-field';
import { Form } from './ui/form';
import { TooltipProvider } from './ui/tooltip';

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}));

jest.mock('@/hooks/use-llm-request', () => ({
  useFetchAllAddedModels: jest.fn(() => ({
    data: [
      { model_id: 'mineru-uuid', provider_name: 'MinerU' },
      { model_id: 'minerunet-uuid', provider_name: 'MinerU.Net' },
      { model_id: 'other-uuid', provider_name: 'Other' },
    ],
    isFetched: true,
    isError: false,
  })),
}));

jest.mock('./model-tree-select', () => ({ buildModelTree: () => [] }));
jest.mock('./tree-select', () => {
  const React = jest.requireActual('react');
  return {
    TreeSelect: React.forwardRef(
      (_: unknown, ref: import('react').ForwardedRef<HTMLButtonElement>) => (
        <button type="button" ref={ref}>
          Parser selector
        </button>
      ),
    ),
  };
});
jest.mock('./paddleocr-options-form-field', () => ({
  PaddleOCROptionsFormField: () => null,
}));

/** Hosts real options in either the PDF setup or default parser form path. */
function Fixture({
  method,
  showOptions = true,
  defaultSelector = false,
  explicit = false,
}: {
  method: string;
  showOptions?: boolean;
  defaultSelector?: boolean;
  explicit?: boolean;
}) {
  const options = explicit
    ? { mineru_formula_enable: false, mineru_table_enable: true }
    : {};
  const form = useForm({
    defaultValues: defaultSelector
      ? { parser_config: { layout_recognize: method, ...options } }
      : { setups: [{ parse_method: method, ...options }] },
  });
  const values = form.watch();

  return (
    <TooltipProvider>
      <Form {...form}>
        <LayoutRecognizeFormField
          name={
            defaultSelector
              ? 'parser_config.layout_recognize'
              : 'setups.0.parse_method'
          }
          ownerTenantId="owner-tenant"
          showMineruOptions={showOptions}
          showPaddleocrOptions={false}
        />
        <output data-testid="form-values">{JSON.stringify(values)}</output>
      </Form>
    </TooltipProvider>
  );
}

describe('MinerU PDF options', () => {
  it.each([
    'mineru-uuid',
    'minerunet-uuid',
    'model@instance@MinerU',
    'model@MinerU.Net',
    'mineru',
  ])('recognizes %s without relying on the model name', (method) => {
    render(<Fixture method={method} />);
    expect(screen.getByText('MinerU Options')).toBeInTheDocument();
    expect(
      screen.getByRole('switch', { name: /Formula Recognition/ }),
    ).toBeChecked();
    expect(
      screen.getByRole('switch', { name: /Table Recognition/ }),
    ).toBeChecked();
    expect(useFetchAllAddedModels).toHaveBeenCalledWith(
      undefined,
      'owner-tenant',
    );
  });

  it('writes each switch under its PDF setup and preserves explicit false', () => {
    render(<Fixture method="mineru-uuid" explicit />);
    const formula = screen.getByRole('switch', { name: /Formula Recognition/ });
    const table = screen.getByRole('switch', { name: /Table Recognition/ });
    expect(formula).not.toBeChecked();
    expect(table).toBeChecked();
    fireEvent.click(table);
    expect(table).not.toBeChecked();
    const values = JSON.parse(screen.getByTestId('form-values').textContent!);
    expect(values.setups[0]).toMatchObject({
      mineru_formula_enable: false,
      mineru_table_enable: false,
    });
    expect(values).not.toHaveProperty('parser_config');
  });

  it('uses the default selector prefix when requested', () => {
    render(<Fixture method="mineru-uuid" defaultSelector explicit />);
    fireEvent.click(screen.getByRole('switch', { name: /Table Recognition/ }));
    const values = JSON.parse(screen.getByTestId('form-values').textContent!);
    expect(values.parser_config).toMatchObject({
      mineru_formula_enable: false,
      mineru_table_enable: false,
    });
  });

  it.each(['other-uuid', 'DeepDOC', 'other-mineru-model@instance@Other'])(
    'does not expose MinerU controls for %s',
    (method) => {
      render(<Fixture method={method} />);
      expect(screen.queryByText('MinerU Options')).not.toBeInTheDocument();
    },
  );

  it('can suppress MinerU options on non-PDF forms', () => {
    render(<Fixture method="mineru-uuid" showOptions={false} />);
    expect(screen.queryByText('MinerU Options')).not.toBeInTheDocument();
  });
});
