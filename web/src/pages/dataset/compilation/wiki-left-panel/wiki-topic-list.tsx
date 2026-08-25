import { Spin } from '@/components/ui/spin';
import { IArtifactTopic } from '@/interfaces/database/dataset';
import { useTranslation } from 'react-i18next';

import { WikiTopicListItem } from './wiki-list-item';

type WikiTopicListProps = {
  topics: IArtifactTopic[];
  loading: boolean;
  hasMore: boolean;
  onSelectTopic: (topic: IArtifactTopic) => void;
};

export function WikiTopicList({
  topics,
  loading,
  hasMore,
  onSelectTopic,
}: WikiTopicListProps) {
  const { t } = useTranslation();

  return (
    <>
      <ul className="space-y-1">
        {topics.map((topic) => (
          <WikiTopicListItem
            key={topic.topic}
            topic={topic}
            onSelect={onSelectTopic}
          />
        ))}
      </ul>
      {loading && (
        <div className="py-4 flex justify-center">
          <Spin size="small" />
        </div>
      )}
      {!loading && !hasMore && topics.length > 0 && (
        <div className="py-2 text-center text-sm text-text-secondary">
          {t('knowledgeList.noMoreData')}
        </div>
      )}
    </>
  );
}
