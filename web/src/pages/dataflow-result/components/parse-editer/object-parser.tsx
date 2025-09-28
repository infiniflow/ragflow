import { cn } from '@/lib/utils';
import { CheckedState } from '@radix-ui/react-checkbox';
import { useCallback, useEffect, useRef, useState } from 'react';
import { ChunkTextMode } from '../../constant';
import styles from '../../index.less';

type IProps = {
  initialValue: {
    key: string;
    type: string;
    value: string;
  };
  isChunck?: boolean;
  handleCheck: (e: CheckedState, index: number) => void;
  unescapeNewlines: (text: string) => string;
  escapeNewlines: (text: string) => string;
  onSave: (data: { value: string; key: string; type: string }) => void;
  className?: string;
  textMode?: ChunkTextMode;
};
export const ObjectContainer = (props: IProps) => {
  const {
    initialValue,
    isChunck,
    unescapeNewlines,
    escapeNewlines,
    onSave,
    className,
    textMode,
  } = props;

  const [content, setContent] = useState(initialValue);

  useEffect(() => {
    setContent(initialValue);
    console.log('initialValue object parse', initialValue);
  }, [initialValue]);

  const [activeEditIndex, setActiveEditIndex] = useState<number | undefined>(
    undefined,
  );
  const editDivRef = useRef<HTMLDivElement>(null);
  const handleEdit = useCallback(
    (e?: any) => {
      console.log(e, e.target.innerText);
      setContent((pre) => ({
        ...pre,
        value: e.target.innerText,
      }));
      setActiveEditIndex(1);
    },
    [setContent, setActiveEditIndex],
  );
  const handleSave = useCallback(
    (e: any) => {
      console.log(e, e.target.innerText);
      const saveData = {
        ...content,
        value: unescapeNewlines(e.target.innerText),
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
            contentEditable={true}
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
              handleEdit(e);
            }}
          >
            {escapeNewlines(content.value)}
          </div>
        )}
      </section>
    </>
  );
};
