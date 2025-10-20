import { cn } from '@/lib/utils';
import { useCallback, useEffect } from 'react';
import { ChunkTextMode } from '../../constant';
import styles from '../../index.less';
import { IChunk } from '../../interface';
import { useParserInit } from './hook';
import { IObjContainerProps } from './interface';
export const ObjectContainer = (props: IObjContainerProps) => {
  const {
    initialValue,
    isChunck,
    onSave,
    className,
    textMode,
    clickChunk,
    isReadonly,
  } = props;

  const {
    content,
    // setContent,
    activeEditIndex,
    setActiveEditIndex,
    editDivRef,
  } = useParserInit({ initialValue });

  const handleEdit = useCallback(() => {
    // setContent((pre) => ({
    //   ...pre,
    //   value: escapeNewlines(e.target.innerText),
    // }));
    setActiveEditIndex(1);
  }, [setActiveEditIndex]);

  const handleSave = useCallback(
    (e: any) => {
      const saveData = {
        ...content,
        value: e.target.textContent,
      };
      onSave(saveData);
      setActiveEditIndex(undefined);
    },
    [content, onSave, setActiveEditIndex],
  );

  useEffect(() => {
    if (activeEditIndex !== undefined && editDivRef.current) {
      editDivRef.current.focus();
      editDivRef.current.textContent = content.value as string;
      editDivRef.current.style.whiteSpace = 'pre-wrap';
    }
  }, [activeEditIndex, content, editDivRef]);

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
            onClick={() => {
              clickChunk(content as unknown as IChunk);
              if (!isReadonly) {
                handleEdit();
              }
            }}
          >
            {content.value as string}
          </div>
        )}
      </section>
    </>
  );
};
