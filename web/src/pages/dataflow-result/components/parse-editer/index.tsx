import { CheckedState } from '@radix-ui/react-checkbox';
import { FormatPreserveEditorProps } from './interface';
import { ArrayContainer } from './json-parser';
import { ObjectContainer } from './object-parser';

const FormatPreserveEditor = ({
  initialValue,
  onSave,
  className,
  isChunck,
  handleCheckboxClick,
  selectedChunkIds,
  textMode,
  clickChunk,
  isReadonly,
}: FormatPreserveEditorProps) => {
  console.log('initialValue', initialValue);

  const escapeNewlines = (text: string) => {
    return text.replace(/\n/g, '\\n');
  };
  const unescapeNewlines = (text: string) => {
    return text.replace(/\\n/g, '\n');
  };
  const handleCheck = (e: CheckedState, id: string | number) => {
    handleCheckboxClick?.(id, e === 'indeterminate' ? false : e);
  };

  return (
    <div className="editor-container">
      {['json', 'chunks'].includes(initialValue.key) && (
        <ArrayContainer
          isReadonly={isReadonly}
          className={className}
          initialValue={initialValue}
          handleCheck={handleCheck}
          selectedChunkIds={selectedChunkIds}
          onSave={onSave}
          escapeNewlines={escapeNewlines}
          unescapeNewlines={unescapeNewlines}
          textMode={textMode}
          isChunck={isChunck}
          clickChunk={clickChunk}
        />
      )}

      {['text', 'html', 'markdown'].includes(initialValue.key) && (
        <ObjectContainer
          isReadonly={isReadonly}
          className={className}
          initialValue={initialValue}
          handleCheck={handleCheck}
          selectedChunkIds={selectedChunkIds}
          onSave={onSave}
          escapeNewlines={escapeNewlines}
          unescapeNewlines={unescapeNewlines}
          textMode={textMode}
          isChunck={isChunck}
          clickChunk={clickChunk}
        />
      )}
    </div>
  );
};

export default FormatPreserveEditor;
