import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { t } from 'i18next';
import { z } from 'zod';

export type OutputType = {
  title: string;
  type?: string;
};

type OutputProps = {
  list: Array<OutputType>;
  isFormRequired?: boolean;
};

export function transferOutputs(outputs: Record<string, any>) {
  return Object.entries(outputs).map(([key, value]) => ({
    title: key,
    type: value?.type,
  }));
}

export const OutputSchema = {
  outputs: z.record(z.any()),
};

export function Output({ list, isFormRequired = false }: OutputProps) {
  return (
    <section className="space-y-2">
      <div className="text-sm">{t('flow.output')}</div>
      <ul>
        {list.map((x, idx) => (
          <li
            key={idx}
            className="bg-background-highlight text-accent-primary rounded-sm px-2 py-1"
          >
            {x.title}: <span className="text-text-secondary">{x.type}</span>
          </li>
        ))}
      </ul>
      {isFormRequired && (
        <RAGFlowFormItem name="outputs" className="hidden">
          <Input></Input>
        </RAGFlowFormItem>
      )}
    </section>
  );
}
