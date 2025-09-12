import { Output, OutputType } from '@/pages/agent/form/components/output';
import { memo } from 'react';
import { initialIterationStartValues } from '../../constant';

const outputs = initialIterationStartValues.outputs;

const outputList = Object.entries(outputs).reduce<OutputType[]>(
  (pre, [key, value]) => {
    pre.push({ title: key, type: value.type });

    return pre;
  },
  [],
);
function IterationStartForm() {
  return (
    <section className="space-y-6 p-4">
      <Output list={outputList}></Output>
    </section>
  );
}

export default memo(IterationStartForm);
