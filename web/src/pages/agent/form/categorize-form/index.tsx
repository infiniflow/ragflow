import { MessageHistoryWindowSizeFormField } from '@/components/message-history-window-size-item';
import { ModelTreeSelectFormField } from '@/components/model-tree-select';
import { Form } from '@/components/ui/form';
import { Separator } from '@/components/ui/separator';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { initialCategorizeValues } from '../../constant';
import { useOwnerTenantId } from '../../context';
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
  const { t } = useTranslation();
  const values = useValues(node);

  const FormSchema = useCreateCategorizeFormSchema();

  const form = useForm({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  const ownerTenantId = useOwnerTenantId();

  return (
    <Form {...form}>
      <FormWrapper>
        <QueryVariable></QueryVariable>
        <ModelTreeSelectFormField
          name="llm_id"
          label={t('chat.model')}
          modelTypes={['chat', 'vision', 'decision']}
          providers={[]}
          ownerTenantId={ownerTenantId}
        />
        <MessageHistoryWindowSizeFormField
          min={0}
        ></MessageHistoryWindowSizeFormField>
        <Separator />
        <DynamicCategorize nodeId={node?.id}></DynamicCategorize>
        <Output list={outputList}></Output>
      </FormWrapper>
    </Form>
  );
}

export default memo(CategorizeForm);
