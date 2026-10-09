import { useMemo } from 'react';
import { useFetchBuiltinPipelines } from '@/hooks/use-agent-request';
import { useTranslation } from 'react-i18next';
import {
  filterBuiltinByKeyword,
  shouldShowBuiltinForRaw,
  toBuiltinListItem,
} from './builtin-pipeline-list';
import { BuiltinPipelineCard } from './builtin-pipeline-card';

// BuiltinPipelineSection renders the read-only built-in pipeline catalog after
// the user's own canvases. It owns the catalog query so the request is issued
// only when this component is mounted (the agents page mounts it solely for the
// "All" and "Pipeline" views), keeping the "fire a query where its data is
// rendered" convention and avoiding a needless request in the Agent /
// compilation-template views.
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
  const { data: builtinData } = useFetchBuiltinPipelines();

  const builtinItems = useMemo(() => {
    if (!shouldShowBuiltinForRaw(rawCategory)) {
      return [];
    }
    return filterBuiltinByKeyword(builtinData?.canvas ?? [], searchString).map(
      toBuiltinListItem,
    );
  }, [rawCategory, builtinData, searchString]);

  if (builtinItems.length === 0) {
    return null;
  }

  return (
    <section className="mt-6" data-testid="builtin-pipeline-section">
      {showDivider && <div className="border-t border-line-divider my-2" />}
      <h2 className="text-sm font-medium text-text-secondary mb-3">
        {t('knowledgeConfiguration.builtInPipelines')}
      </h2>
      {builtinItems.map((b) => (
        <BuiltinPipelineCard key={b.id} data={b} />
      ))}
    </section>
  );
}
