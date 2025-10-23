import { t } from 'i18next';

export type OutputType = {
  title: string;
  type?: string;
};

type OutputProps = {
  list: Array<OutputType>;
};

export function transferOutputs(outputs: Record<string, any>) {
  return Object.entries(outputs).map(([key, value]) => ({
    title: key,
    type: value?.type,
  }));
}

export function Output({ list }: OutputProps) {
  return (
    <section className="space-y-2">
      <div>{t('flow.output')}</div>
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
    </section>
  );
}
