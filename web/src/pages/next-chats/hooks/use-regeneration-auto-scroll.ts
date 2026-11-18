import { IMessage } from '@/interfaces/database/chat';
import {
  RefObject,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
} from 'react';

interface UseRegenerationAutoScrollOptions {
  conversationId: string;
  messages: IMessage[];
  isStreaming: boolean;
  containerRef: RefObject<HTMLDivElement>;
}

export function useRegenerationAutoScroll({
  conversationId,
  messages,
  isStreaming,
  containerRef,
}: UseRegenerationAutoScrollOptions) {
  const shouldFollowRef = useRef(false);

  const startAutoScroll = useCallback(() => {
    shouldFollowRef.current = true;
  }, []);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const stopFollowing = () => {
      shouldFollowRef.current = false;
    };
    container.addEventListener('wheel', stopFollowing, { passive: true });
    container.addEventListener('touchstart', stopFollowing, { passive: true });
    container.addEventListener('pointerdown', stopFollowing);

    return () => {
      shouldFollowRef.current = false;
      container.removeEventListener('wheel', stopFollowing);
      container.removeEventListener('touchstart', stopFollowing);
      container.removeEventListener('pointerdown', stopFollowing);
    };
  }, [conversationId, containerRef]);

  // ResizeObserver can lag behind a streamed answer, leaving the virtualizer's
  // size cache below the actual DOM height. Follow the real scroll height for
  // this turn until it finishes or the user deliberately scrolls away.
  useLayoutEffect(() => {
    if (!shouldFollowRef.current) return;

    const container = containerRef.current;
    container?.scrollTo({ top: container.scrollHeight });

    if (!isStreaming) {
      shouldFollowRef.current = false;
    }
  }, [messages, isStreaming, containerRef]);

  return { startAutoScroll };
}
