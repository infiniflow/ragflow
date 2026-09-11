import { render, screen, fireEvent } from '@testing-library/react';
import { expect, jest, test } from '@jest/globals';
import '@testing-library/jest-dom/jest-globals';
import { useForm } from 'react-hook-form';
import { Form } from '@/components/ui/form';
import { TooltipProvider } from '@/components/ui/tooltip';
import { FXMacroDataWidgets } from './index';
import operations from './operations.json';
import { getToolOperatorName } from '../../log-sheet/tool-name';

jest.mock('../../hooks/use-form-values', () => ({ useFormValues: () => ({}) }));
jest.mock('../../hooks/use-watch-form-change', () => ({
  useWatchFormChange: () => undefined,
}));
jest.mock('../components/output', () => ({ Output: () => null }));
jest.mock('../../constant', () => ({
  initialFXMacroDataValues: { outputs: {} },
}));

test.each([
  'fxmacrodata_release_calendar',
  'fxmacrodata_mcp_ping',
  'FXMacroData',
])('native tool timeline identifies %s', (name) =>
  expect(getToolOperatorName(name)).toBe('FXMacroData'),
);

function Harness({
  operation = 'release_calendar',
  agentTool = false,
}: {
  operation?: string;
  agentTool?: boolean;
}) {
  const form = useForm({
    defaultValues: {
      operation,
      arguments: operation === 'release_calendar' ? { currency: 'usd' } : {},
      use_credentials: false,
      timeout: 30,
    },
  });
  const values = form.watch();
  return (
    <TooltipProvider>
      <Form {...form}>
        <FXMacroDataWidgets agentTool={agentTool} />
        <output data-testid="values">{JSON.stringify(values)}</output>
      </Form>
    </TooltipProvider>
  );
}

test('public calendar is usable with a real typed currency field and provider link', () => {
  render(<Harness />);
  expect(screen.getByDisplayValue('usd')).toBeInTheDocument();
  expect(
    screen.getByRole('link', { name: 'FXMacroData API reference' }),
  ).toHaveAttribute(
    'href',
    expect.stringContaining('https://fxmacrodata.com/'),
  );
  expect(
    JSON.parse(screen.getByTestId('values').textContent || '{}')
      .use_credentials,
  ).toBe(false);
  fireEvent.change(screen.getByDisplayValue('usd'), {
    target: { value: 'eur' },
  });
  expect(
    JSON.parse(screen.getByTestId('values').textContent || '{}').arguments
      .currency,
  ).toBe('eur');
});

test.each(operations.map((operation) => [operation.name]))(
  'native form renders operation %s without a credential field',
  (operation) => {
    const { container } = render(<Harness operation={operation} />);
    expect(
      screen.getByRole('link', { name: 'FXMacroData API reference' }),
    ).toBeInTheDocument();
    expect(container.querySelector('input[type="password"]')).toBeNull();
    expect(
      JSON.parse(screen.getByTestId('values').textContent || '{}').operation,
    ).toBe(operation);
  },
);

test('agent form exposes configuration while leaving execution arguments to the model schema', () => {
  render(<Harness agentTool />);
  expect(screen.queryByDisplayValue('usd')).toBeNull();
  expect(
    screen.getByText(/agent supplies the selected operation/),
  ).toBeInTheDocument();
});

test('numeric timeout remains editable through the normal form field', () => {
  render(<Harness operation="ping" />);
  fireEvent.change(screen.getByDisplayValue('30'), { target: { value: '45' } });
  expect(
    JSON.parse(screen.getByTestId('values').textContent || '{}').timeout,
  ).toBe(45);
});

test('nullable pagination distinguishes an omitted value, a number and explicit null', () => {
  render(<Harness operation="indicator_history" />);
  const values = () =>
    JSON.parse(screen.getByTestId('values').textContent || '{}').arguments;
  const nullable = screen.getByRole('checkbox', { name: 'Send null for page' });
  const input = screen.getByLabelText('Page');
  expect(values()).not.toHaveProperty('page');
  fireEvent.change(input, { target: { value: '2' } });
  expect(values().page).toBe(2);
  fireEvent.click(nullable);
  expect(values()).toHaveProperty('page', null);
  expect(screen.getByDisplayValue('null')).toBeDisabled();
  fireEvent.click(nullable);
  expect(values()).not.toHaveProperty('page');
  fireEvent.change(screen.getByLabelText('Page'), { target: { value: '3' } });
  expect(values().page).toBe(3);
  fireEvent.change(screen.getByLabelText('Page'), { target: { value: '' } });
  expect(values()).not.toHaveProperty('page');
});

test('invalid replacement JSON replaces persisted arguments and correction recovers', () => {
  render(<Harness operation="mcp_plot_visual_artifact" />);
  const values = () =>
    JSON.parse(screen.getByTestId('values').textContent || '{}').arguments;
  const input = screen.getByLabelText('series');
  fireEvent.change(input, {
    target: { value: '[{"source":"announcements"}]' },
  });
  expect(values().series).toEqual([{ source: 'announcements' }]);
  fireEvent.change(input, { target: { value: '[{"source":' } });
  expect(values().series).toBe('[{"source":');
  expect(input).toHaveValue('[{"source":');
  expect(
    screen.getByText('Enter valid JSON for this structured field.'),
  ).toBeInTheDocument();
  fireEvent.change(input, { target: { value: '[]' } });
  expect(values().series).toEqual([]);
  expect(
    screen.queryByText('Enter valid JSON for this structured field.'),
  ).toBeNull();
});
