import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { t } from 'i18next';
import { PropsWithChildren } from 'react';
import { z } from 'zod';

export type OutputType = {
  title: string;
  type?: string;
  description?: string;
};

type OutputProps = {
  list: Array<OutputType>;
  isFormRequired?: boolean;
} & PropsWithChildren;

export function transferOutputs(outputs: Record<string, any> | undefined) {
  if (!outputs) {
    return [];
  }
  return Object.entries(outputs).map(([key, value]) => ({
    title: key,
    type: value?.type,
  }));
}

export const OutputSchema = {
  outputs: z.record(z.any()),
};

export function Output({
  list,
  isFormRequired = false,
  children,
}: OutputProps) {
  return (
    <section className="space-y-2">
      <div className="text-sm flex items-center justify-between">
        {t('flow.output')} <span>{children}</span>
      </div>
      <ul className="space-y-1">
        {list.map((x, idx) => (
          <li
            key={idx}
            className="bg-background-highlight text-accent-primary rounded-sm px-2 py-1"
          >
            <div>
              {x.title}: <span className="text-text-secondary">{x.type}</span>
            </div>
            {x.description ? (
              <p className="mt-0.5 text-xs text-text-secondary">{x.description}</p>
            ) : null}
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
