import { Checkbox } from '@/components/ui/checkbox';
import { cn } from '@/lib/utils';
import { useCallback, useEffect } from 'react';
import { ChunkTextMode } from '../../constant';
import styles from '../../index.less';
import { useParserInit } from './hook';
import { IJsonContainerProps } from './interface';
export const parserKeyMap = {
  json: 'text',
  chunks: 'text',
};

export const ArrayContainer = (props: IJsonContainerProps) => {
  const {
    initialValue,
    isChunck,
    handleCheck,
    selectedChunkIds,
    unescapeNewlines,
    escapeNewlines,
    onSave,
    className,
    textMode,
    clickChunk,
    isReadonly,
  } = props;

  const {
    content,
    setContent,
    activeEditIndex,
    setActiveEditIndex,
    editDivRef,
  } = useParserInit({ initialValue });

  const handleEdit = useCallback(
    (e?: any, index?: number) => {
      setContent((pre) => ({
        ...pre,
        value: pre.value.map((item, i) => {
          if (i === index) {
            return {
              ...item,
              [parserKeyMap[content.key]]: unescapeNewlines(e.target.innerText),
            };
          }
          return item;
        }),
      }));
      setActiveEditIndex(index);
    },
    [setContent, setActiveEditIndex],
  );
  const handleSave = useCallback(
    (e: any) => {
      const saveData = {
        ...content,
        value: content.value?.map((item, index) => {
          if (index === activeEditIndex) {
            return {
              ...item,
              [parserKeyMap[content.key]]: e.target.innerText,
            };
          } else {
            return item;
          }
        }),
      };
      onSave(saveData);
      setActiveEditIndex(undefined);
    },
    [content, onSave],
  );

  useEffect(() => {
    if (activeEditIndex !== undefined && editDivRef.current) {
      editDivRef.current.focus();
      editDivRef.current.textContent = escapeNewlines(
        content.value[activeEditIndex][parserKeyMap[content.key]],
      );
    }
  }, [activeEditIndex, content]);

  return (
    <>
      {content.value?.map((item, index) => {
        if (item[parserKeyMap[content.key]] === '') {
          return null;
        }
        return (
          <section
            key={index}
            className={
              isChunck
                ? 'bg-bg-card my-2 p-2 rounded-lg flex gap-1 items-start'
                : ''
            }
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
                  'text-text-secondary overflow-auto scrollbar-auto whitespace-pre-wrap w-full',
                  {
                    [styles.contentEllipsis]:
                      textMode === ChunkTextMode.Ellipse,
                  },
                )}
                key={index}
                onClick={(e) => {
                  clickChunk(item);
                  if (!isReadonly) {
                    handleEdit(e, index);
                  }
                }}
              >
                {escapeNewlines(item[parserKeyMap[content.key]])}
              </div>
            )}
          </section>
        );
      })}
    </>
  );
};
