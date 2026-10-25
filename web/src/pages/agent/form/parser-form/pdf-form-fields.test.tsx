import { fireEvent, render, screen, within } from '@testing-library/react';
import { TooltipProvider } from '@/components/ui/tooltip';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { FormProvider, useForm, useWatch } from 'react-hook-form';
import { PdfFormFields } from './pdf-form-fields';

jest.mock('@/components/layout-recognize-form-field', () =>
  jest.requireActual('../../../../components/layout-recognize-form-field'),
);
jest.mock('@/hooks/use-llm-request', () => ({
  useFetchAllAddedModels: jest.fn(),
}));
jest.mock('@/hooks/common-hooks', () => ({
  useTranslate: () => ({ t: (key: string) => key }),
}));
jest.mock('../../context', () => ({ useOwnerTenantId: () => 'canvas-owner' }));
jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const Models = useFetchAllAddedModels as jest.Mock;
const Model = {
  model_id: 'tenant-ocr-model',
  name: 'Receipt OCR',
  provider_name: 'PaddleOCR',
  provider_id: 'provider',
  instance_name: 'local',
  instance_id: 'instance',
  model_type: ['ocr'],
};
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
beforeEach(() => {
  Models.mockReturnValue({ data: [Model], isFetched: true, isError: false });
});

function PdfForm({ method = 'DeepDOC' }: { method?: string }) {
  const form = useForm({
    defaultValues: {
      setups: [
        {
          parse_method: method,
          remove_toc: true,
          remove_header_footer: false,
          pages: [],
        },
      ],
    },
  });
  const setups = useWatch({ control: form.control, name: 'setups' });
  return (
    <TooltipProvider>
      <FormProvider {...form}>
        <PdfFormFields prefix="setups.0" />
        <output data-testid="saved-setup">{JSON.stringify(setups[0])}</output>
      </FormProvider>
    </TooltipProvider>
  );
}

function cleanupSwitch(label: string) {
  const item = screen.getByText(label).closest('label')!
    .parentElement!.parentElement!;
  return within(item).getByRole('switch');
}

function expectCleanupVisible(visible: boolean) {
  for (const label of ['flow.removeToc', 'flow.removeHeaderFooter']) {
    if (visible) {
      expect(screen.getByText(label)).toBeInTheDocument();
    } else {
      expect(screen.queryByText(label)).not.toBeInTheDocument();
    }
  }
  expect(screen.getByText('flow.parserMethod')).toBeInTheDocument();
  expect(screen.getByText('knowledgeDetails.pageRanges')).toBeInTheDocument();
}

describe('PDF cleanup controls', () => {
  it.each([
    'Plain Text',
    'plain_text',
    'plaintext',
    'Docling',
    'OpenDataLoader',
    'TCADP Parser',
    'MonkeyOCRv2',
    'mineru',
    'paddleocr',
    'somark',
    'tenant-ocr-model',
    'Receipt OCR@local@PaddleOCR',
    'Receipt OCR@PaddleOCR',
  ])('does not offer ignored cleanup settings for %s', (method) => {
    render(<PdfForm method={method} />);
    expectCleanupVisible(false);
  });

  it.each(['DeepDOC', 'deepdoc', 'deepdoc_ee', 'registered_analyzer', ''])(
    'preserves cleanup for the DeepDoc route %s',
    (method) => {
      render(<PdfForm method={method} />);
      expectCleanupVisible(true);
    },
  );

  it('retains edited values when changing methods through the real selector', () => {
    render(<PdfForm />);
    fireEvent.click(cleanupSwitch('flow.removeToc'));
    fireEvent.click(cleanupSwitch('flow.removeHeaderFooter'));
    fireEvent.click(screen.getByText('DeepDOC').closest('button')!);
    fireEvent.click(within(screen.getByRole('dialog')).getByText('Docling'));
    expectCleanupVisible(false);
    expect(
      JSON.parse(screen.getByTestId('saved-setup').textContent!),
    ).toMatchObject({
      parse_method: 'Docling',
      remove_toc: false,
      remove_header_footer: true,
    });
    fireEvent.click(screen.getByText('Docling').closest('button')!);
    fireEvent.click(within(screen.getByRole('dialog')).getByText('DeepDOC'));
    expectCleanupVisible(true);
    expect(cleanupSwitch('flow.removeToc')).not.toBeChecked();
    expect(cleanupSwitch('flow.removeHeaderFooter')).toBeChecked();
  });

  it.each([
    { data: [], isFetched: false, isError: false },
    { data: [], isFetched: true, isError: true },
  ])('does not expose controls before resolving a saved model: %j', (state) => {
    Models.mockReturnValue(state);
    render(<PdfForm method="tenant-ocr-model" />);
    expectCleanupVisible(false);
  });

  it('keeps controls hidden when the model query finishes', () => {
    Models.mockReturnValue({ data: [], isFetched: false, isError: false });
    const { rerender } = render(<PdfForm method="tenant-ocr-model" />);
    expectCleanupVisible(false);
    Models.mockReturnValue({ data: [Model], isFetched: true, isError: false });
    rerender(<PdfForm method="tenant-ocr-model" />);
    expectCleanupVisible(false);
    expect(useFetchAllAddedModels).toHaveBeenCalledWith(
      undefined,
      'canvas-owner',
    );
  });

  it('restores a registered analyzer after model selections are resolved', () => {
    Models.mockReturnValue({ data: [], isFetched: false, isError: false });
    const { rerender } = render(<PdfForm method="registered_analyzer" />);
    expectCleanupVisible(false);
    Models.mockReturnValue({ data: [], isFetched: true, isError: false });
    rerender(<PdfForm method="registered_analyzer" />);
    expectCleanupVisible(true);
  });

  it('keeps the built-in DeepDoc controls available if the model query fails', () => {
    Models.mockReturnValue({ data: [], isFetched: true, isError: true });
    render(<PdfForm />);
    expectCleanupVisible(true);
  });
});
