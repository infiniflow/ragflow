import { fireEvent, render, screen } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';
import { FileType } from '@/constants/file';
import message from '@/components/ui/message';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import en from '@/locales/en';
import zh from '@/locales/zh';
import { ImageFormFields } from './image-form-fields';

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { warning: jest.fn() },
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => (key === 'flow.imageOcrLocal' ? 'DeepDoc' : key),
  }),
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
  RAGFlowFormItem: ({ name, children, description }: any) => {
    const {
      Controller,
      useFormContext,
    }: typeof import('react-hook-form') = require('react-hook-form');
    const { control } = useFormContext();
    return (
      <>
        <Controller
          name={name}
          control={control}
          render={({ field }) =>
            typeof children === 'function' ? children(field) : children
          }
        />
        {description}
      </>
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
      <pre data-testid="saved-values">{JSON.stringify(form.watch())}</pre>
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
    expect(screen.getByTestId('image-ocr-model')).toHaveTextContent('DeepDoc');
    expect(useFetchAllAddedModels).toHaveBeenCalledWith('ocr', 'canvas-owner');
  });

  it('names the built-in OCR model DeepDoc in English and Chinese', () => {
    expect(en.translation.flow.imageOcrLocal).toBe('DeepDoc');
    expect(zh.translation.flow.imageOcrLocal).toBe('DeepDoc');
  });

  it('switches from an enrolled model back to DeepDoc without persisting a sentinel', () => {
    (useFetchAllAddedModels as jest.Mock).mockReturnValue({
      data: models,
      isFetched: true,
      isError: false,
    });
    render(<ParserImageForm />);
    fireEvent.click(screen.getByTestId('image-ocr-model'));
    fireEvent.click(screen.getByText('PP-OCRv6'));
    expect(screen.getByTestId('saved-values')).toHaveTextContent(
      '"ocr_model_id":"local-b"',
    );
    fireEvent.click(screen.getByTestId('image-ocr-model'));
    fireEvent.click(screen.getByText('DeepDoc'));
    expect(screen.getByTestId('image-ocr-model')).toHaveTextContent('DeepDoc');
    expect(screen.getByTestId('saved-values')).toHaveTextContent(
      '"ocr_model_id":""',
    );
    fireEvent.click(screen.getByTestId('image-ocr-model'));
    fireEvent.click(screen.getByText('PaddleOCR-VL-1.5'));
    expect(screen.getByTestId('saved-values')).toHaveTextContent(
      '"ocr_model_id":"cloud-b"',
    );
  });

  it('distinguishes a successful empty list from a failed request', () => {
    render(<ParserImageForm />);
    expect(screen.getByRole('status')).toHaveTextContent(
      'flow.imageOcrNoModels',
    );
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId('image-ocr-model'));
    expect(screen.getAllByText('DeepDoc')).toHaveLength(2);
  });

  it('shows a request failure and a missing saved model after loading ends', () => {
    const modelQuery = useFetchAllAddedModels as jest.Mock;
    modelQuery.mockReturnValue({
      data: [],
      isFetched: false,
      isError: false,
    });
    const { rerender } = render(<ParserImageForm modelId="deleted-model" />);
    expect(
      screen.queryByText('Missing: deleted-model'),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    modelQuery.mockReturnValue({
      data: [],
      isFetched: true,
      isError: true,
    });
    rerender(<ParserImageForm modelId="deleted-model" />);
    expect(screen.getByRole('alert')).toHaveTextContent(
      'flow.imageOcrModelsError',
    );
    expect(screen.getByTestId('image-ocr-model')).toHaveTextContent(
      'Missing: deleted-model',
    );
    expect(screen.getByTestId('saved-values')).toHaveTextContent(
      '"ocr_model_id":"deleted-model"',
    );
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId('image-ocr-model'));
    fireEvent.click(screen.getByText('DeepDoc'));
    expect(screen.getByTestId('saved-values')).toHaveTextContent(
      '"ocr_model_id":""',
    );
  });

  it('preserves cached models and the selected ID when refetch fails', () => {
    (useFetchAllAddedModels as jest.Mock).mockReturnValue({
      data: models,
      isFetched: true,
      isError: true,
    });
    render(<ParserImageForm modelId="cloud-b" />);
    expect(screen.getByRole('alert')).toHaveTextContent(
      'flow.imageOcrModelsError',
    );
    expect(screen.getByTestId('image-ocr-model')).toHaveTextContent(
      'PaddleOCR-VL-1.5',
    );
    expect(screen.getByTestId('saved-values')).toHaveTextContent(
      '"ocr_model_id":"cloud-b"',
    );
    expect(screen.queryByText('Missing: cloud-b')).not.toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('keeps DeepDoc selected when the first model request fails', () => {
    (useFetchAllAddedModels as jest.Mock).mockReturnValue({
      data: [],
      isFetched: true,
      isError: true,
    });
    render(<ParserImageForm />);
    expect(screen.getByRole('alert')).toHaveTextContent(
      'flow.imageOcrModelsError',
    );
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.getByTestId('image-ocr-model')).toHaveTextContent('DeepDoc');
    expect(screen.getByTestId('saved-values')).toHaveTextContent(
      '"ocr_model_id":""',
    );
    fireEvent.click(screen.getByTestId('image-ocr-model'));
    expect(screen.getAllByText('DeepDoc')).toHaveLength(2);
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
