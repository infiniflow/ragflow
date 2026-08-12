import { CompilationTemplateFormField } from '@/components/compilation-template-form-field';
import { LargeModelFormField } from '@/components/large-model-form-field';
import { SwitchFormField } from '@/components/switch-fom-field';
import { Form } from '@/components/ui/form';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { initialCompilationValues } from '../../constant/pipeline';
import { useOwnerTenantId } from '../../context';
import { useFormChangeCallback } from '../../hooks/use-form-change-callback';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';

export const FormSchema = z.object({
  compilation_template_group_id: z.string().optional(),
  llm_id: z.string().optional(),
  plan: z.boolean(),
});

export type CompilationFormSchemaType = z.infer<typeof FormSchema>;

const outputList = buildOutputList(initialCompilationValues.outputs);

const CompilationForm = ({
  node,
  onValuesChange,
  hideOutputs,
}: INextOperatorForm) => {
  const defaultValues = useFormValues(initialCompilationValues, node);
  const ownerTenantId = useOwnerTenantId();
  const { t } = useTranslate('setting');

  const form = useForm<CompilationFormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useWatchFormChange(node?.id, form);
  useFormChangeCallback(form, onValuesChange);

  return (
    <Form {...form}>
      <FormWrapper>
        <CompilationTemplateFormField name="compilation_template_group_id"></CompilationTemplateFormField>
        <LargeModelFormField
          name="llm_id"
          ownerTenantId={ownerTenantId}
        ></LargeModelFormField>
        <SwitchFormField
          name="plan"
          label={t('plan')}
          tooltip={t('planTip')}
        ></SwitchFormField>
      </FormWrapper>
      {!hideOutputs && (
        <div className="p-5">
          <Output list={outputList}></Output>
        </div>
      )}
    </Form>
  );
};

export default memo(CompilationForm);
