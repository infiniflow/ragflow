import { cn } from '@/lib/utils';
import { useRef } from 'react';

import { useX6Graph } from './hooks/use-x6-graph';
import { type TimelineX6GraphProps } from './types';

export function TimelineX6Graph({
  data,
  show = true,
  onNodeClick,
}: TimelineX6GraphProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  useX6Graph(containerRef, data, onNodeClick);

  return (
    <div
      ref={containerRef}
      className={cn('w-full h-full min-h-0', !show && 'hidden')}
    />
  );
}

export default TimelineX6Graph;
