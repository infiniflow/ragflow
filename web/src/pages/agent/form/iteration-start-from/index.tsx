import { Output, OutputType } from '@/pages/agent/form/components/output';
import { initialIterationStartValues } from '../../constant';

const outputs = initialIterationStartValues.outputs;

const outputList = Object.entries(outputs).reduce<OutputType[]>(
  (pre, [key, value]) => {
    pre.push({ title: key, type: value.type });

    return pre;
  },
  [],
);
const IterationStartForm = () => {
  return (
    <section className="space-y-6 p-4">
      <Output list={outputList}></Output>
    </section>
  );
};

export default IterationStartForm;
