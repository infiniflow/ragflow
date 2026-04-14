import { Checkbox } from '@/components/ui/checkbox';
import { cn } from '@/lib/utils';
import { isArray } from 'lodash';
import { useCallback, useEffect, useMemo } from 'react';
import { ChunkTextMode } from '../../constant';
import styles from '../../index.module.less';
import { IChunk } from '../../interface';
import { useParserInit } from './hook';
import { IJsonContainerProps } from './interface';
export const parserKeyMap = {
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
  } = props;

  const { content, activeEditIndex, setActiveEditIndex, editDivRef } =
    useParserInit({ initialValue });

  const parserKey = useMemo(() => {
    const key =
      content.key === 'chunks' && content.params.field_name
        ? content.params.field_name
        : parserKeyMap[content.key as keyof typeof parserKeyMap];
    return key;
  }, [content]);

  const handleEdit = useCallback(
    (e?: any, index?: number) => {
      setActiveEditIndex(index);
    },
    [setActiveEditIndex],
  );

  const handleSave = useCallback(
    (e: any) => {
      if (Array.isArray(content.value)) {
        const saveData = {
          ...content,
          value: content.value?.map((item, index) => {
            if (index === activeEditIndex) {
              return {
                ...item,
                [parserKey]: e.target.textContent || '',
              };
            } else {
              return item;
            }
          }),
        };
        onSave(saveData as any);
      }
      setActiveEditIndex(undefined);
    },
    [content, onSave, activeEditIndex, parserKey, setActiveEditIndex],
  );

  useEffect(() => {
    if (activeEditIndex !== undefined && editDivRef.current) {
      editDivRef.current.focus();
      if (typeof content.value !== 'string') {
        editDivRef.current.textContent =
          content.value[activeEditIndex][parserKey];
      }
    }
  }, [editDivRef, activeEditIndex, content, parserKey]);

  return (
    <>
      {isArray(content.value) &&
        content.value?.map((item, index) => {
          if (
            item[parserKeyMap[content.key as keyof typeof parserKeyMap]] === ''
          ) {
            return null;
          }
          return (
            <section
              key={index}
              className={cn(
                isChunck
                  ? 'bg-bg-card my-2 p-2 rounded-lg flex gap-1 items-start'
                  : '',
                activeEditIndex === index && isChunck ? 'bg-bg-title' : '',
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
