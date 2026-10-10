import { fireEvent, render, screen } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';
import { VisionEnhancementFormFields } from './vision-form-fields';

jest.mock('@/hooks/use-llm-request', () => ({
  useFetchDefaultModelDictionary: () => ({ img2txt_id: 'model-A' }),
}));

jest.mock('../../context', () => ({ useOwnerTenantId: () => 'tenant-1' }));

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
        render={({ field }) => children(field)}
      />
    );
  },
}));

jest.mock('@/components/model-tree-select', () => ({
  ModelTypeMap: { img2txt_id: ['image2text'] },
  ModelTreeSelectFormField: ({ name }: any) => {
    const {
      Controller,
      useFormContext,
    }: typeof import('react-hook-form') = require('react-hook-form');
    const { control } = useFormContext();
    return (
      <Controller
        name={name}
        control={control}
        render={({ field }) => (
          <select aria-label="Vision model" {...field}>
            <option value="">Default</option>
            <option value="model-A">A</option>
            <option value="model-B">B</option>
          </select>
        )}
      />
    );
  },
}));

function ParserVisionForm({ model = '' }: { model?: string }) {
  const form = useForm({
    defaultValues: { enable_vision_enhancement: false, vlm: { llm_id: model } },
  });
  return (
    <FormProvider {...form}>
      <VisionEnhancementFormFields />
    </FormProvider>
  );
}

describe('global vision model selection', () => {
  it('selects default A on first enable and preserves user-selected B across toggles', () => {
    render(<ParserVisionForm />);
    fireEvent.click(screen.getByRole('switch'));
    expect(screen.getByLabelText('Vision model')).toHaveValue('model-A');

    fireEvent.change(screen.getByLabelText('Vision model'), {
      target: { value: 'model-B' },
    });
    fireEvent.click(screen.getByRole('switch'));
    expect(screen.queryByLabelText('Vision model')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('switch'));
    expect(screen.getByLabelText('Vision model')).toHaveValue('model-B');
  });

  it('keeps saved model B when enhancement is enabled again', () => {
    render(<ParserVisionForm model="model-B" />);
    fireEvent.click(screen.getByRole('switch'));
    expect(screen.getByLabelText('Vision model')).toHaveValue('model-B');
  });
});
