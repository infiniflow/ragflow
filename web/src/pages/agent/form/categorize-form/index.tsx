import { FormContainer } from '@/components/form-container';
import { LargeModelFormField } from '@/components/large-model-form-field';
import { MessageHistoryWindowSizeFormField } from '@/components/message-history-window-size-item';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { initialCategorizeValues } from '../../constant';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';
import DynamicCategorize from './dynamic-categorize';
import { useCreateCategorizeFormSchema } from './use-form-schema';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

const outputList = buildOutputList(initialCategorizeValues.outputs);

function CategorizeForm({ node }: INextOperatorForm) {
  const values = useValues(node);

  const FormSchema = useCreateCategorizeFormSchema();

  const form = useForm({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <QueryVariable></QueryVariable>
          <LargeModelFormField></LargeModelFormField>
        </FormContainer>
        <MessageHistoryWindowSizeFormField></MessageHistoryWindowSizeFormField>
        <DynamicCategorize nodeId={node?.id}></DynamicCategorize>
        <Output list={outputList}></Output>
      </FormWrapper>
    </Form>
  );
}

export default memo(CategorizeForm);
