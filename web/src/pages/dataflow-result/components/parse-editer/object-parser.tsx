import { cn } from '@/lib/utils';
import { useCallback, useEffect } from 'react';
import { ChunkTextMode } from '../../constant';
import styles from '../../index.less';
import { useParserInit } from './hook';
import { IObjContainerProps } from './interface';
export const ObjectContainer = (props: IObjContainerProps) => {
  const {
    initialValue,
    isChunck,
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
    (e?: any) => {
      setContent((pre) => ({
        ...pre,
        value: escapeNewlines(e.target.innerText),
      }));
      setActiveEditIndex(1);
    },
    [setContent, setActiveEditIndex],
  );

  const handleSave = useCallback(
    (e: any) => {
      const saveData = {
        ...content,
        value: e.target.innerText,
      };
      onSave(saveData);
      setActiveEditIndex(undefined);
    },
    [content, onSave],
  );

  useEffect(() => {
    if (activeEditIndex !== undefined && editDivRef.current) {
      editDivRef.current.focus();
      editDivRef.current.textContent = escapeNewlines(content.value);
    }
  }, [activeEditIndex, content, escapeNewlines]);

  return (
    <>
      <section
        className={
          isChunck
            ? 'bg-bg-card my-2 p-2 rounded-lg flex gap-1 items-start'
            : ''
        }
      >
        {activeEditIndex && (
          <div
            ref={editDivRef}
            contentEditable={!isReadonly}
            onBlur={handleSave}
            className={cn(
              'w-full bg-transparent text-text-secondary border-none focus-visible:border-none focus-visible:ring-0 focus-visible:ring-offset-0 focus-visible:outline-none p-0',
              className,
            )}
          />
        )}
        {!activeEditIndex && (
          <div
            className={cn(
              'text-text-secondary overflow-auto scrollbar-auto whitespace-pre-wrap w-full',
              {
                [styles.contentEllipsis]: textMode === ChunkTextMode.Ellipse,
              },
            )}
            onClick={(e) => {
              clickChunk(content);
              if (!isReadonly) {
                handleEdit(e);
              }
            }}
          >
            {escapeNewlines(content.value)}
          </div>
        )}
      </section>
    </>
  );
};
