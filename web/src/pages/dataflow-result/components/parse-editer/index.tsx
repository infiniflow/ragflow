import { Checkbox } from '@/components/ui/checkbox';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import { CheckedState } from '@radix-ui/react-checkbox';
import { useEffect, useState } from 'react';
import { ChunkTextMode } from '../../constant';
import styles from '../../index.less';
interface FormatPreserveEditorProps {
  initialValue: {
    key: string;
    type: string;
    value: Array<{ [key: string]: string }>;
  };
  onSave: (value: any) => void;
  className?: string;
  isSelect?: boolean;
  isDelete?: boolean;
  isChunck?: boolean;
  handleCheckboxClick?: (id: string | number, checked: boolean) => void;
  selectedChunkIds?: string[];
  textMode?: ChunkTextMode;
}
const FormatPreserveEditor = ({
  initialValue,
  onSave,
  className,
  isChunck,
  handleCheckboxClick,
  selectedChunkIds,
  textMode,
}: FormatPreserveEditorProps) => {
  const [content, setContent] = useState(initialValue);
  // const [isEditing, setIsEditing] = useState(false);
  const [activeEditIndex, setActiveEditIndex] = useState<number | undefined>(
    undefined,
  );
  console.log('initialValue', initialValue);

  useEffect(() => {
    setContent(initialValue);
  }, [initialValue]);
  const handleEdit = (e?: any, index?: number) => {
    console.log(e, index, content);
    if (content.key === 'json') {
      console.log(e, e.target.innerText);
      setContent((pre) => ({
        ...pre,
        value: pre.value.map((item, i) => {
          if (i === index) {
            return {
              ...item,
              [Object.keys(item)[0]]: e.target.innerText,
            };
          }
          return item;
        }),
      }));
      setActiveEditIndex(index);
    }
  };

  const handleChange = (e: any) => {
    if (content.key === 'json') {
      setContent((pre) => ({
        ...pre,
        value: pre.value.map((item, i) => {
          if (i === activeEditIndex) {
            return {
              ...item,
              [Object.keys(item)[0]]: e.target.value,
            };
          }
          return item;
        }),
      }));
    } else {
      setContent(e.target.value);
    }
  };

  const escapeNewlines = (text: string) => {
    return text.replace(/\n/g, '\\n');
  };
  const unescapeNewlines = (text: string) => {
    return text.replace(/\\n/g, '\n');
  };

  const handleSave = () => {
    const saveData = {
      ...content,
      value: content.value?.map((item) => {
        return { ...item, text: unescapeNewlines(item.text) };
      }),
    };
    onSave(saveData);
    setActiveEditIndex(undefined);
  };
  const handleCheck = (e: CheckedState, id: string | number) => {
    handleCheckboxClick?.(id, e === 'indeterminate' ? false : e);
  };
  return (
    <div className="editor-container">
      {/* {isEditing && content.key === 'json' ? (
        <Textarea
          className={cn(
            'w-full h-full bg-transparent text-text-secondary border-none focus-visible:border-none focus-visible:ring-0 focus-visible:ring-offset-0 focus-visible:outline-none min-h-6 p-0',
            className,
          )}
          value={content.value}
          onChange={handleChange}
          onBlur={handleSave}
          autoSize={{ maxRows: 100 }}
          autoFocus
        />
      ) : (
        <>
          {content.key === 'json' && */}
      {content.value?.map((item, index) => (
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
            <Textarea
              key={'t' + index}
              className={cn(
                'w-full bg-transparent text-text-secondary border-none focus-visible:border-none focus-visible:ring-0 focus-visible:ring-offset-0 focus-visible:outline-none !h-6 min-h-6 p-0',
                className,
              )}
              value={escapeNewlines(content.value[index].text)}
              onChange={handleChange}
              onBlur={handleSave}
              autoSize={{ maxRows: 100, minRows: 1 }}
              autoFocus
            />
          )}
          {activeEditIndex !== index && (
            <div
              className={cn(
                'text-text-secondary overflow-auto scrollbar-auto whitespace-pre-wrap w-full',
                {
                  [styles.contentEllipsis]: textMode === ChunkTextMode.Ellipse,
                },
              )}
              key={index}
              onClick={(e) => {
                handleEdit(e, index);
              }}
            >
              {escapeNewlines(item.text)}
            </div>
          )}
        </section>
      ))}
      {/* {content.key !== 'json' && (
            <pre
              className="text-text-secondary overflow-auto scrollbar-auto"
              onClick={handleEdit}
            >
            </pre>
          )} 
        </>
      )}*/}
    </div>
  );
};

export default FormatPreserveEditor;
