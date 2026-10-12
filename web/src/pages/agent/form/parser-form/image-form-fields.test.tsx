import { fireEvent, render, screen } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';
import { FileType } from '@/constants/file';
import message from '@/components/ui/message';
import { ImageFormFields } from './image-form-fields';

jest.mock('@/components/ui/message', () => ({
  __esModule: true,
  default: { warning: jest.fn() },
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
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
}: {
  visionEnabled?: boolean;
  ocrEnabled?: boolean;
}) {
  const form = useForm({
    defaultValues: {
      enable_vision_enhancement: visionEnabled,
      vlm: { llm_id: '', system_prompt: '' },
      setups: [{ fileFormat: FileType.Image, ocr_enabled: ocrEnabled }],
    },
  });
  return (
    <FormProvider {...form}>
      <ImageFormFields prefix="setups.0" />
    </FormProvider>
  );
}

beforeEach(() => {
  warningMock.mockClear();
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
