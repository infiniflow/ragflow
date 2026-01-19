import { SliderInputFormField } from '@/components/slider-input-form-field';
import { Form } from '@/components/ui/form';
import { FormLayout } from '@/constants/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { initialLoopValues } from '../../constant';
import { INextOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { DynamicVariables } from './dynamic-variables';
import { LoopTerminationCondition } from './loop-termination-condition';
import { FormSchema, LoopFormSchemaType } from './schema';
import { useFormValues } from './use-values';
import { useWatchFormChange } from './use-watch-form-change';

function LoopForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialLoopValues, node);
  const { t } = useTranslation();

  const form = useForm<LoopFormSchemaType>({
    defaultValues: defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <DynamicVariables
          name="loop_variables"
          label={t('flow.loopVariables')}
          nodeId={node?.id}
        ></DynamicVariables>
        <LoopTerminationCondition
          label={t('flow.loopTerminationCondition')}
          nodeId={node?.id}
        ></LoopTerminationCondition>
        <SliderInputFormField
          min={1}
          max={100}
          name="maximum_loop_count"
          label={t('flow.maximumLoopCount')}
          layout={FormLayout.Vertical}
        ></SliderInputFormField>
      </FormWrapper>
    </Form>
  );
}

export default memo(LoopForm);
