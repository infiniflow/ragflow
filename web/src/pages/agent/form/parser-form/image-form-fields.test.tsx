import { fireEvent, render, screen } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';
import { FileType } from '@/constants/file';
import message from '@/components/ui/message';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { ImageFormFields } from './image-form-fields';

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { warning: jest.fn() },
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock('@/hooks/use-llm-request', () => ({
  useFetchAllAddedModels: jest.fn(() => ({
    data: [],
    isFetched: true,
    isError: false,
  })),
}));

jest.mock('../../context', () => ({
  useOwnerTenantId: () => 'canvas-owner',
}));

jest.mock('@/components/llm-select/llm-label', () => ({
  __esModule: true,
  default: () => null,
  MissingModelLabel: ({ value }: { value: string }) => (
    <span>Missing: {value}</span>
  ),
}));

jest.mock('@/components/ragflow-form', () => ({
  RAGFlowFormItem: ({ name, children }: any) => {
    const {
      Controller,
      useFormContext,
    }: typeof import('react-hook-form') = require('react-hook-form');
    const { control } = useFormContext();
    return (
      <Controller
        name={name}
        control={control}
        render={({ field }) =>
          typeof children === 'function' ? children(field) : children
        }
      />
    );
  },
}));

const warningMock = message.warning as jest.Mock;

function ParserImageForm({
  visionEnabled = false,
  ocrEnabled = true,
  modelId = '',
}: {
  visionEnabled?: boolean;
  ocrEnabled?: boolean;
  modelId?: string;
}) {
  const form = useForm({
    defaultValues: {
      enable_vision_enhancement: visionEnabled,
      vlm: { llm_id: '', system_prompt: '' },
      setups: [
        {
          fileFormat: FileType.Image,
          ocr_enabled: ocrEnabled,
          ocr_model_id: modelId,
        },
      ],
    },
  });
  return (
    <FormProvider {...form}>
      <ImageFormFields prefix="setups.0" />
      <output data-testid="saved-values">{JSON.stringify(form.watch())}</output>
    </FormProvider>
  );
}

beforeEach(() => {
  warningMock.mockClear();
  (useFetchAllAddedModels as jest.Mock).mockReturnValue({
    data: [],
    isFetched: true,
    isError: false,
  });
});

describe('image OCR model selection', () => {
  const models = [
    {
      model_id: 'local-b',
      name: 'PP-OCRv6',
      provider_name: 'PaddleOCR.local',
      instance_name: 'Local B',
      model_type: ['ocr'],
    },
    {
      model_id: 'cloud-b',
      name: 'PaddleOCR-VL-1.5',
      provider_name: 'PaddleOCR',
      instance_name: 'Cloud B',
      model_type: ['ocr'],
    },
    {
      model_id: 'mineru',
      name: 'Unsupported OCR',
      provider_name: 'MinerU',
      instance_name: 'Other',
      model_type: ['ocr'],
    },
    {
      model_id: 'vision',
      name: 'Not an OCR model',
      provider_name: 'PaddleOCR',
      instance_name: 'Vision',
      model_type: ['vision'],
    },
  ];

  it('defaults to local OCR and fetches the canvas owner models', () => {
    render(<ParserImageForm />);
    expect(screen.getByTestId('image-ocr-model')).toHaveTextContent(
      'flow.imageOcrLocal',
    );
    expect(useFetchAllAddedModels).toHaveBeenCalledWith('ocr', 'canvas-owner');
  });

  it('offers only supported enrolled OCR models and saves their IDs', () => {
    (useFetchAllAddedModels as jest.Mock).mockReturnValue({
      data: models,
      isFetched: true,
    });
    render(<ParserImageForm />);
    fireEvent.click(screen.getByTestId('image-ocr-model'));
    expect(screen.queryByText('Unsupported OCR')).not.toBeInTheDocument();
    expect(screen.queryByText('Not an OCR model')).not.toBeInTheDocument();
    fireEvent.click(screen.getByText('PP-OCRv6'));
    expect(screen.getByTestId('saved-values')).toHaveTextContent(
      '"ocr_model_id":"local-b"',
    );
  });

  it('preserves the selected model while OCR is off and restores it on re-enable', () => {
    (useFetchAllAddedModels as jest.Mock).mockReturnValue({
      data: models,
      isFetched: true,
    });
    render(<ParserImageForm visionEnabled modelId="cloud-b" />);
    fireEvent.click(screen.getByRole('switch'));
    expect(screen.queryByTestId('image-ocr-model')).not.toBeInTheDocument();
    expect(screen.getByTestId('saved-values')).toHaveTextContent(
      '"ocr_model_id":"cloud-b"',
    );
    fireEvent.click(screen.getByRole('switch'));
    expect(screen.getByTestId('image-ocr-model')).toHaveTextContent(
      'PaddleOCR-VL-1.5',
    );
  });

  it('keeps a deleted model visible instead of silently choosing local OCR', () => {
    render(<ParserImageForm modelId="deleted-model" />);
    expect(screen.getByTestId('image-ocr-model')).toHaveTextContent(
      'Missing: deleted-model',
    );
  });
});

describe('image OCR switch guard', () => {
  it('blocks turning OCR off while vision enhancement is off', () => {
    render(<ParserImageForm visionEnabled={false} ocrEnabled={true} />);

    fireEvent.click(screen.getByRole('switch'));

    expect(warningMock).toHaveBeenCalledWith('flow.imageTextSourceRequired');
    expect(screen.getByRole('switch')).toHaveAttribute('aria-checked', 'true');
  });

  it('allows turning OCR off while vision enhancement is on', () => {
    render(<ParserImageForm visionEnabled={true} ocrEnabled={true} />);

    fireEvent.click(screen.getByRole('switch'));

    expect(warningMock).not.toHaveBeenCalled();
    expect(screen.getByRole('switch')).toHaveAttribute('aria-checked', 'false');
  });

  it('always allows turning OCR on', () => {
    render(<ParserImageForm visionEnabled={false} ocrEnabled={false} />);

    fireEvent.click(screen.getByRole('switch'));

    expect(warningMock).not.toHaveBeenCalled();
    expect(screen.getByRole('switch')).toHaveAttribute('aria-checked', 'true');
  });
});
