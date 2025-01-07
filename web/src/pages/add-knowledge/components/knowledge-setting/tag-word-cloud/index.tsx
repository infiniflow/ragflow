import { useFetchTagList } from '@/hooks/knowledge-hooks';
import { useCallback, useEffect, useRef } from 'react';
import WordCloud from 'wordcloud';

export function TagWorkCloud() {
  const domRef = useRef<HTMLDivElement>(null);
  const { list } = useFetchTagList();

  const renderWordCloud = useCallback(() => {
    if (domRef.current && WordCloud.isSupported) {
      WordCloud(domRef.current, {
        list: list,
        gridSize: 12,
        weightFactor: 16,
        color: 'random-dark',
        backgroundColor: '#f0f0f0',
      });
    }
  }, [list]);

  useEffect(() => {
    renderWordCloud();

    return () => {
      // stop the renderring
      WordCloud.stop();
    };
  }, [renderWordCloud]);

  return <div ref={domRef} className="w-full h-[38vh]"></div>;
}
