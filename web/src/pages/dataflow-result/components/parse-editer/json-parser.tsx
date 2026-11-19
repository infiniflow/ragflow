import { Checkbox } from '@/components/ui/checkbox';
import { cn } from '@/lib/utils';
import { isArray } from 'lodash';
import {
  type FocusEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
} from 'react';
import { ChunkTextMode } from '../../constant';
import styles from '../../index.module.less';
import { IChunk } from '../../interface';
import { useParserInit } from './hook';
import { IJsonContainerProps } from './interface';
const parserKeyMap = {
  json: 'text',
  chunks: 'text',
} as const;

export const ArrayContainer = (props: IJsonContainerProps) => {
  const {
    initialValue,
    isChunck,
    handleCheck,
    selectedChunkIds,
    onSave,
    className,
    textMode,
    clickChunk,
    isReadonly,
    newChunkIndex,
  } = props;

  const { content, activeEditIndex, setActiveEditIndex, editDivRef } =
    useParserInit({ initialValue });

  const sectionRefs = useRef<(HTMLElement | null)[]>([]);

  const parserKey = useMemo(() => {
    const key =
      content.key === 'chunks' && content.params.field_name
        ? content.params.field_name
        : parserKeyMap[content.key];
    return key;
  }, [content]);

  const handleEdit = useCallback(
    (_event?: unknown, index?: number) => {
      setActiveEditIndex(index);
    },
    [setActiveEditIndex],
  );

  const handleSave = useCallback(
    (event: FocusEvent<HTMLDivElement>) => {
      const text = event.currentTarget.textContent ?? '';
      onSave({
        ...content,
        value: content.value.map((item, index) =>
          index === activeEditIndex ? { ...item, [parserKey]: text } : item,
        ),
      });
      setActiveEditIndex(undefined);
    },
    [content, onSave, activeEditIndex, parserKey, setActiveEditIndex],
  );

  useEffect(() => {
    if (activeEditIndex !== undefined && editDivRef.current) {
      editDivRef.current.focus();
      editDivRef.current.textContent =
        content.value[activeEditIndex]?.[parserKey] ?? '';
    }
  }, [editDivRef, activeEditIndex, content, parserKey]);

  useEffect(() => {
    if (newChunkIndex === undefined) return;
    const target = sectionRefs.current[newChunkIndex];
    if (!target) return;

    // Scroll only the nearest scrollable ancestor so the whole page doesn't shift
    let scrollContainer: HTMLElement | null = target.parentElement;
    while (scrollContainer) {
      const { overflowY } = window.getComputedStyle(scrollContainer);
      if (overflowY === 'auto' || overflowY === 'scroll') break;
      scrollContainer = scrollContainer.parentElement;
    }

    if (scrollContainer) {
      const containerRect = scrollContainer.getBoundingClientRect();
      const targetRect = target.getBoundingClientRect();
      const offsetTop =
        targetRect.top - containerRect.top + scrollContainer.scrollTop;
      const targetCenter = offsetTop + target.clientHeight / 2;
      scrollContainer.scrollTo({
        top: Math.max(0, targetCenter - scrollContainer.clientHeight / 2),
        behavior: 'smooth',
      });
    }
  }, [newChunkIndex, content]);

  return (
    <>
      {isArray(content.value) &&
        content.value?.map((item, index) => {
          if (item[parserKeyMap[content.key]] === '' || !item[parserKey]) {
            return null;
          }
          return (
            <section
              key={index}
              ref={(el) => {
                sectionRefs.current[index] = el;
              }}
              className={cn(
                isChunck
                  ? 'bg-bg-card my-2 p-2 rounded-lg flex gap-1 items-start transition-shadow duration-300'
                  : '',
                activeEditIndex === index && isChunck ? 'bg-bg-title' : '',
                newChunkIndex === index && isChunck
                  ? 'ring-2 ring-accent-primary'
                  : '',
              )}
            >
              {isChunck && !isReadonly && (
                <Checkbox
                  onCheckedChange={(e) => {
                    handleCheck(e, index);
                  }}
                  checked={selectedChunkIds?.some(
                    (id) => id.toString() === index.toString(),
                  )}
                ></Checkbox>
              )}
              {activeEditIndex === index && (
                <div
                  ref={editDivRef}
                  contentEditable={!isReadonly}
                  onBlur={handleSave}
                  className={cn(
                    'w-full bg-transparent text-text-secondary border-none focus-visible:border-none focus-visible:ring-0 focus-visible:ring-offset-0 focus-visible:outline-none p-0',

                    className,
                  )}
                ></div>
              )}
              {activeEditIndex !== index && (
                <div
                  className={cn(
                    'text-text-secondary overflow-auto scrollbar-auto w-full min-h-3',
                    {
                      [styles.contentEllipsis]:
                        textMode === ChunkTextMode.Ellipse,
                    },
                  )}
                  key={index}
                  onClick={(e) => {
                    clickChunk(item as unknown as IChunk);
                    console.log('clickChunk', item, index);
                    if (!isReadonly) {
                      handleEdit(e, index);
                    }
                  }}
                >
                  {item[parserKey]}
                </div>
              )}
            </section>
          );
        })}
    </>
  );
};
