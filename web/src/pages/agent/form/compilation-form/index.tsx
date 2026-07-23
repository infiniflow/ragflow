import { CompilationTemplateFormField } from '@/components/compilation-template-form-field';
import { LargeModelFormField } from '@/components/large-model-form-field';
import { LlmSettingSchema } from '@/components/llm-setting-items/next';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { initialCompilationValues } from '../../constant/pipeline';
import { useOwnerTenantId } from '../../context';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';

export const FormSchema = z.object({
  compilation_template_group_ids: z.string().optional(),
  ...LlmSettingSchema,
});

export type CompilationFormSchemaType = z.infer<typeof FormSchema>;

const outputList = buildOutputList(initialCompilationValues.outputs);

const CompilationForm = ({ node }: INextOperatorForm) => {
  const defaultValues = useFormValues(initialCompilationValues, node);
  const ownerTenantId = useOwnerTenantId();

  const form = useForm<CompilationFormSchemaType>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <LargeModelFormField
          ownerTenantId={ownerTenantId}
        ></LargeModelFormField>
        <CompilationTemplateFormField name="compilation_template_group_ids"></CompilationTemplateFormField>
        <Output list={outputList}></Output>
      </FormWrapper>
    </Form>
  );
};

export default memo(CompilationForm);
