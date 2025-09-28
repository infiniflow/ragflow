import { Checkbox } from '@/components/ui/checkbox';
import { cn } from '@/lib/utils';
import { CheckedState } from '@radix-ui/react-checkbox';
import { useCallback, useEffect, useRef, useState } from 'react';
import { ChunkTextMode } from '../../constant';
import styles from '../../index.less';
export const parserKeyMap = {
  json: 'text',
  chunks: 'content_with_weight',
};
type IProps = {
  initialValue: {
    key: keyof typeof parserKeyMap;
    type: string;
    value: {
      [key: string]: string;
    }[];
  };
  isChunck?: boolean;
  handleCheck: (e: CheckedState, index: number) => void;
  selectedChunkIds: string[] | undefined;
  unescapeNewlines: (text: string) => string;
  escapeNewlines: (text: string) => string;
  onSave: (data: {
    value: {
      text: string;
    }[];
    key: string;
    type: string;
  }) => void;
  className?: string;
  textMode?: ChunkTextMode;
};
export const ArrayContainer = (props: IProps) => {
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
  } = props;

  const [content, setContent] = useState(initialValue);

  useEffect(() => {
    setContent(initialValue);
    console.log('initialValue json parse', initialValue);
  }, [initialValue]);

  const [activeEditIndex, setActiveEditIndex] = useState<number | undefined>(
    undefined,
  );
  const editDivRef = useRef<HTMLDivElement>(null);
  const handleEdit = useCallback(
    (e?: any, index?: number) => {
      console.log(e, e.target.innerText);
      setContent((pre) => ({
        ...pre,
        value: pre.value.map((item, i) => {
          if (i === index) {
            return {
              ...item,
              [parserKeyMap[content.key]]: e.target.innerText,
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
      console.log(e, e.target.innerText);
      const saveData = {
        ...content,
        value: content.value?.map((item, index) => {
          if (index === activeEditIndex) {
            return {
              ...item,
              [parserKeyMap[content.key]]: unescapeNewlines(e.target.innerText),
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
      editDivRef.current.textContent =
        content.value[activeEditIndex][parserKeyMap[content.key]];
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
            {isChunck && (
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
                contentEditable={true}
                onBlur={handleSave}
                //   onKeyUp={handleChange}
                // dangerouslySetInnerHTML={{
                //   __html: DOMPurify.sanitize(
                //     escapeNewlines(
                //       content.value[index][parserKeyMap[content.key]],
                //     ),
                //   ),
                // }}
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
                  handleEdit(e, index);
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
