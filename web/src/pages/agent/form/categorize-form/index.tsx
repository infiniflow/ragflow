import { LargeModelFormField } from '@/components/large-model-form-field';
import { MessageHistoryWindowSizeFormField } from '@/components/message-history-window-size-item';
import { Form } from '@/components/ui/form';
import { INextOperatorForm } from '../../interface';
import { DynamicInputVariable } from '../components/next-dynamic-input-variable';
import DynamicCategorize from './dynamic-categorize';

const CategorizeForm = ({ form, node }: INextOperatorForm) => {
  return (
    <Form {...form}>
      <form
        className="space-y-6 p-5 overflow-auto max-h-[76vh]"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <DynamicInputVariable node={node}></DynamicInputVariable>
        <LargeModelFormField></LargeModelFormField>
        <MessageHistoryWindowSizeFormField></MessageHistoryWindowSizeFormField>
        <DynamicCategorize nodeId={node?.id}></DynamicCategorize>
      </form>
    </Form>
  );
};

export default CategorizeForm;
