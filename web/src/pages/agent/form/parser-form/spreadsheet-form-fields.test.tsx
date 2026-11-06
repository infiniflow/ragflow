import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { TooltipProvider } from '@/components/ui/tooltip';
import { FormProvider, useForm, useWatch } from 'react-hook-form';
import { SpreadsheetFormFields } from './spreadsheet-form-fields';

jest.mock('@/components/layout-recognize-form-field', () =>
  jest.requireActual('../../../../components/layout-recognize-form-field'),
);

jest.mock('@/hooks/use-llm-request', () => ({
  useFetchAllAddedModels: () => ({
    data: [
      {
        model_id: 'vision-model',
        name: 'Spreadsheet vision model',
        provider_name: 'OpenAI',
        provider_id: 'provider-1',
        instance_name: 'test-instance',
        instance_id: 'instance-1',
        model_type: ['vision'],
      },
    ],
    isFetched: true,
    isError: false,
  }),
}));

jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));

jest.mock('../../context', () => ({ useOwnerTenantId: () => 'tenant-1' }));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const OriginalResizeObserver = Object.getOwnPropertyDescriptor(
  globalThis,
  'ResizeObserver',
);
const OriginalScrollIntoView = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'scrollIntoView',
);

beforeAll(() => {
  Object.defineProperty(globalThis, 'ResizeObserver', {
    configurable: true,
    value: class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  });
  HTMLElement.prototype.scrollIntoView = jest.fn();
});

afterAll(() => {
  if (OriginalResizeObserver) {
    Object.defineProperty(globalThis, 'ResizeObserver', OriginalResizeObserver);
  } else {
    Reflect.deleteProperty(globalThis, 'ResizeObserver');
  }
  if (OriginalScrollIntoView) {
    Object.defineProperty(
      HTMLElement.prototype,
      'scrollIntoView',
      OriginalScrollIntoView,
    );
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView');
  }
});

function SpreadsheetForm({ method = 'DeepDOC' }: { method?: string }) {
  const form = useForm({
    defaultValues: { setups: [{ parse_method: method }] },
  });
  const setups = useWatch({ control: form.control, name: 'setups' });
  return (
    <TooltipProvider>
      <FormProvider {...form}>
        <SpreadsheetFormFields prefix="setups.0" />
        <output data-testid="saved-setup">{JSON.stringify(setups[0])}</output>
      </FormProvider>
    </TooltipProvider>
  );
}

describe('spreadsheet parser methods', () => {
  it('preserves a saved model value until a supported method is selected', async () => {
    render(<SpreadsheetForm method="vision-model" />);
    expect(JSON.parse(screen.getByTestId('saved-setup').textContent!)).toEqual({
      parse_method: 'vision-model',
    });
    fireEvent.click(screen.getByText('vision-model').closest('button')!);
    expect(screen.queryByText('Spreadsheet vision model')).toBeNull();
    fireEvent.click(screen.getByText('DeepDOC'));
    await waitFor(() => {
      expect(
        JSON.parse(screen.getByTestId('saved-setup').textContent!),
      ).toEqual({
        parse_method: 'DeepDOC',
      });
    });
  });

  it('offers only supported methods even when vision models are available', () => {
    render(<SpreadsheetForm />);
    fireEvent.click(screen.getByText('DeepDOC').closest('button')!);

    expect(screen.queryByText('Spreadsheet vision model')).toBeNull();
    expect(
      screen.getAllByRole('option').map((option) => option.textContent),
    ).toEqual(['DeepDOC', 'TCADP Parser']);
  });

  it('saves TCADP selection and shows its configuration only while selected', async () => {
    render(<SpreadsheetForm />);
    expect(screen.queryByText('flow.tableResultType')).toBeNull();
    fireEvent.click(screen.getByText('DeepDOC').closest('button')!);
    fireEvent.click(screen.getByText('TCADP Parser'));

    expect(screen.getByText('flow.tableResultType')).toBeInTheDocument();
    expect(
      screen.getByText('flow.markdownImageResponseType'),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(
        JSON.parse(screen.getByTestId('saved-setup').textContent!),
      ).toEqual({
        parse_method: 'TCADP Parser',
        table_result_type: '1',
        markdown_image_response_type: '1',
      });
    });

    fireEvent.click(screen.getByText('TCADP Parser').closest('button')!);
    fireEvent.click(screen.getByText('DeepDOC'));
    expect(screen.queryByText('flow.tableResultType')).toBeNull();
    await waitFor(() => {
      expect(
        JSON.parse(screen.getByTestId('saved-setup').textContent!).parse_method,
      ).toBe('DeepDOC');
    });
  });
});
