import { TopNFormField } from '@/components/top-n-item';
import { Form } from '@/components/ui/form';
import { INextOperatorForm } from '../../interface';
import { DynamicInputVariable } from '../components/next-dynamic-input-variable';

const BaiduForm = ({ form, node }: INextOperatorForm) => {
  return (
    <Form {...form}>
      <form
        className="space-y-6"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <DynamicInputVariable node={node}></DynamicInputVariable>
        <TopNFormField></TopNFormField>
      </form>
    </Form>
  );
};

export default BaiduForm;
