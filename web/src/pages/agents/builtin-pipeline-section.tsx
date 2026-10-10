import { useTranslation } from 'react-i18next';
import { CardContainer } from '@/components/card-container';
import { useBuiltinItems } from './builtin-pipeline-list';
import { BuiltinPipelineCard } from './builtin-pipeline-card';

// BuiltinPipelineSection renders the read-only built-in pipeline catalog after
// the user's own canvases. It owns the catalog query so the request is issued
// only when this component is mounted (the agents page mounts it solely for the
// "All" and "Pipeline" views), keeping the "fire a query where its data is
// rendered" convention and avoiding a needless request in the Agent /
// compilation-template views. The cards use the same responsive CardContainer
// grid as the user items so the section does not stretch into one long column.
export function BuiltinPipelineSection({
  rawCategory,
  searchString,
  showDivider,
}: {
  rawCategory?: string | string[] | Record<string, string[]>;
  searchString?: string;
  showDivider?: boolean;
}) {
  const { t } = useTranslation();
  const { items: builtinItems } = useBuiltinItems(rawCategory, searchString);

  if (builtinItems.length === 0) {
    return null;
  }

  // col-span-full lets the section span the full width of the parent grid so
  // its own CardContainer grid lays the cards out in multiple columns.
  return (
    <section
      className="mt-6 col-span-full"
      data-testid="builtin-pipeline-section"
    >
      {showDivider && <div className="border-t border-line-divider my-2" />}
      <h2 className="text-sm font-medium text-text-secondary mb-3">
        {t('knowledgeConfiguration.builtInPipelines')}
      </h2>
      <CardContainer>
        {builtinItems.map((b) => (
          <BuiltinPipelineCard key={b.id} data={b} />
        ))}
      </CardContainer>
    </section>
  );
}
