import {
  useFetchArtifactList,
  useFetchArtifactTopicList,
} from '@/hooks/use-knowledge-request';
import { IArtifactTopic } from '@/interfaces/database/dataset';
import { useDebounce } from 'ahooks';
import { useCallback, useMemo, useRef, useState } from 'react';

export type WikiPageType = 'concept' | 'entity';

export function useWikiNavigation() {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [searchString, setSearchString] = useState('');
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });
  const [selectedTopic, setSelectedTopic] = useState<IArtifactTopic | null>(
    null,
  );

  const {
    topics,
    loading: topicLoading,
    handleScroll: handleTopicScroll,
    hasMore: topicHasMore,
  } = useFetchArtifactTopicList({
    keywords: debouncedSearchString,
  });

  const {
    artifacts,
    loading: artifactLoading,
    handleScroll: handleArtifactScroll,
    hasMore: artifactHasMore,
  } = useFetchArtifactList({
    keywords: debouncedSearchString,
    topic: selectedTopic?.topic,
    enabled: !!selectedTopic,
  });

  const handleSearchChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setSearchString(e.target.value);
    },
    [],
  );

  const resetScroll = useCallback(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = 0;
    }
  }, []);

  const handleSelectTopic = useCallback(
    (topic: IArtifactTopic) => {
      setSelectedTopic(topic);
      resetScroll();
    },
    [resetScroll],
  );

  const handleBackToTopics = useCallback(() => {
    setSelectedTopic(null);
    resetScroll();
  }, [resetScroll]);

  const handleScroll = useCallback(
    (e: React.UIEvent<HTMLDivElement>) => {
      if (selectedTopic) {
        handleArtifactScroll(e);
      } else {
        handleTopicScroll(e);
      }
    },
    [selectedTopic, handleArtifactScroll, handleTopicScroll],
  );

  const loading = selectedTopic ? artifactLoading : topicLoading;
  const hasMore = selectedTopic ? artifactHasMore : topicHasMore;

  return useMemo(
    () => ({
      scrollRef,
      searchString,
      debouncedSearchString,
      selectedTopic,
      topics,
      artifacts,
      loading,
      hasMore,
      handleSearchChange,
      handleSelectTopic,
      handleBackToTopics,
      handleScroll,
    }),
    [
      searchString,
      debouncedSearchString,
      selectedTopic,
      topics,
      artifacts,
      loading,
      hasMore,
      handleSearchChange,
      handleSelectTopic,
      handleBackToTopics,
      handleScroll,
    ],
  );
}
