import { Spin } from '@/components/ui/spin';
import { IArtifact } from '@/interfaces/database/dataset';
import { useTranslation } from 'react-i18next';

import { WikiArtifactListItem } from './wiki-list-item';

type WikiArtifactListProps = {
  artifacts: IArtifact[];
  selectedArtifact: IArtifact | null;
  loading: boolean;
  hasMore: boolean;
  onSelectArtifact: (artifact: IArtifact) => void;
};

export function WikiArtifactList({
  artifacts,
  selectedArtifact,
  loading,
  hasMore,
  onSelectArtifact,
}: WikiArtifactListProps) {
  const { t } = useTranslation();

  return (
    <>
      <ul className="space-y-1">
        {artifacts.map((item) => (
          <WikiArtifactListItem
            key={item.slug}
            item={item}
            isSelected={selectedArtifact?.slug === item.slug}
            onSelect={onSelectArtifact}
          />
        ))}
      </ul>
      {loading && (
        <div className="py-4 flex justify-center">
          <Spin size="small" />
        </div>
      )}
      {!loading && !hasMore && artifacts.length > 0 && (
        <div className="py-2 text-center text-sm text-text-secondary">
          {t('knowledgeList.noMoreData')}
        </div>
      )}
    </>
  );
}
