import ArtifactForceGraph from '@/components/artifact-force-graph';
import {
  SelectWithSearch,
  SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { useFetchArtifactGraph } from '@/hooks/use-knowledge-request';
import { IArtifact, IArtifactGraphEntity } from '@/interfaces/database/dataset';
import { IFetchArtifactGraphRequestParams } from '@/interfaces/request/knowledge';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

const GraphSearchTopN = 12;

const mapNodeToValue = (node: IArtifactGraphEntity) => ({
  slug: node.slug,
  title: node.name,
  page_type: node.type,
});

type WikiGraphPanelProps = {
  selectedArtifact: IArtifact | null;
  onSelectArtifact: (artifact: IArtifact) => void;
  onClearArtifact: () => void;
};

export function WikiGraphPanel({
  selectedArtifact,
  onSelectArtifact,
  onClearArtifact,
}: WikiGraphPanelProps) {
  const { t } = useTranslation();
  const [graphKeywords, setGraphKeywords] = useState('');

  const graphParams = useMemo<IFetchArtifactGraphRequestParams | undefined>(
    () =>
      graphKeywords
        ? { keywords: graphKeywords, top_n: GraphSearchTopN }
        : undefined,
    [graphKeywords],
  );
  const { data } = useFetchArtifactGraph(graphParams);

  const entityOptions = useMemo<SelectWithSearchFlagOptionType[]>(
    () =>
      data.entities.map((entity) => ({
        label: entity.name,
        value: entity.slug,
        keywords: [entity.name, ...entity.aliases],
      })),
    [data.entities],
  );

  // Only refill the select when selectedArtifact is a graph entity, to avoid showing the raw slug
  const selectedEntitySlug = data.entities.some(
    (entity) => entity.slug === selectedArtifact?.slug,
  )
    ? (selectedArtifact?.slug ?? '')
    : '';

  const handleSelectEntity = useCallback(
    (slug: string) => {
      if (!slug) {
        setGraphKeywords('');
        onClearArtifact();
        return;
      }
      const entity = data.entities.find((item) => item.slug === slug);
      if (entity) {
        onSelectArtifact(mapNodeToValue(entity));
      }
    },
    [data.entities, onSelectArtifact, onClearArtifact],
  );

  const handleNoMatchEnter = useCallback((keywords: string) => {
    setGraphKeywords(keywords);
  }, []);

  return (
    <div className="flex h-full flex-col gap-3">
      <SelectWithSearch
        options={entityOptions}
        value={selectedEntitySlug || graphKeywords}
        onChange={handleSelectEntity}
        placeholder={t('knowledgeCompilation.searchEntity')}
        allowClear
        triggerClassName="w-96 max-w-full"
        onNoMatchEnter={handleNoMatchEnter}
        disableAutoSelectOnEnter
      />
      <ArtifactForceGraph
        data={data}
        show
        mapNodeToValue={mapNodeToValue}
        onNodeClick={onSelectArtifact}
        highlightNodeId={selectedArtifact?.slug}
        totalEntities={data.total_entities}
        returnedEntities={data.returned_entities}
      />
    </div>
  );
}
