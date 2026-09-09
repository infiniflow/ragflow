import {
  useFetchArtifactList,
  useFetchArtifactTopicList,
} from '@/hooks/use-knowledge-request';
import { IArtifactTopic } from '@/interfaces/database/dataset';
import { useDebounce } from 'ahooks';
import { useCallback, useMemo, useRef, useState } from 'react';

export type WikiPageType = 'concept' | 'entity' | 'topic';

const normalizeTopicPath = (topic: string) =>
  topic
    .split('/')
    .map((segment) => segment.trim())
    .filter(Boolean)
    .join('/');

const topicLeaf = (topic: string) => {
  const segments = topic.split('/');
  return segments[segments.length - 1] ?? topic;
};

function getTopicChildren(topics: IArtifactTopic[], parentPath: string) {
  const normalizedParent = normalizeTopicPath(parentPath);
  const prefix = normalizedParent ? `${normalizedParent}/` : '';
  const children = new Map<string, number>();

  topics.forEach((topic) => {
    const path = normalizeTopicPath(topic.topic);
    if (!path.startsWith(prefix) || path === normalizedParent) {
      return;
    }

    const remainder = path.slice(prefix.length);
    const child = remainder.split('/')[0];
    const childPath = `${prefix}${child}`;
    children.set(
      childPath,
      (children.get(childPath) ?? 0) + (topic.page_count ?? 0),
    );
  });

  return Array.from(children, ([path, pageCount]) => ({
    topic: path,
    title: topicLeaf(path),
    slug: path,
    page_count: pageCount,
  })).sort((a, b) => a.title.localeCompare(b.title));
}

export function useWikiNavigation() {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [searchString, setSearchString] = useState('');
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });
  const [selectedTopicPath, setSelectedTopicPath] = useState<string | null>(
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

  const visibleTopics = useMemo(
    () => getTopicChildren(topics, selectedTopicPath ?? ''),
    [topics, selectedTopicPath],
  );

  const selectedTopic = useMemo<IArtifactTopic | null>(() => {
    if (!selectedTopicPath) {
      return null;
    }
    return {
      topic: selectedTopicPath,
      title: topicLeaf(selectedTopicPath),
      slug: selectedTopicPath,
    };
  }, [selectedTopicPath]);

  const showArtifacts = Boolean(
    selectedTopicPath && visibleTopics.length === 0,
  );

  const {
    artifacts,
    loading: artifactLoading,
    handleScroll: handleArtifactScroll,
    hasMore: artifactHasMore,
  } = useFetchArtifactList({
    keywords: debouncedSearchString,
    topic: showArtifacts ? (selectedTopicPath ?? undefined) : undefined,
    enabled: showArtifacts,
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
      setSelectedTopicPath(normalizeTopicPath(topic.topic));
      resetScroll();
    },
    [resetScroll],
  );

  const handleSelectTopicPath = useCallback(
    (path: string | null) => {
      setSelectedTopicPath(path ? normalizeTopicPath(path) : null);
      resetScroll();
    },
    [resetScroll],
  );

  const handleBackToTopics = useCallback(() => {
    handleSelectTopicPath(null);
  }, [handleSelectTopicPath]);

  const handleScroll = useCallback(
    (e: React.UIEvent<HTMLDivElement>) => {
      if (showArtifacts) {
        handleArtifactScroll(e);
      } else {
        handleTopicScroll(e);
      }
    },
    [showArtifacts, handleArtifactScroll, handleTopicScroll],
  );

  const loading = showArtifacts ? artifactLoading : topicLoading;
  const hasMore = showArtifacts ? artifactHasMore : topicHasMore;

  return useMemo(
    () => ({
      scrollRef,
      searchString,
      debouncedSearchString,
      selectedTopic,
      selectedTopicPath,
      visibleTopics,
      showArtifacts,
      artifacts,
      loading,
      hasMore,
      handleSearchChange,
      handleSelectTopic,
      handleSelectTopicPath,
      handleBackToTopics,
      handleScroll,
    }),
    [
      searchString,
      debouncedSearchString,
      selectedTopic,
      selectedTopicPath,
      visibleTopics,
      showArtifacts,
      artifacts,
      loading,
      hasMore,
      handleSearchChange,
      handleSelectTopic,
      handleSelectTopicPath,
      handleBackToTopics,
      handleScroll,
    ],
  );
}
