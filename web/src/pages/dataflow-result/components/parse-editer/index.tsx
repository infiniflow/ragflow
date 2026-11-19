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
  newChunkIndex,
}: FormatPreserveEditorProps) => {
  const handleCheck = (e: CheckedState, id: string | number) => {
    handleCheckboxClick?.(id, e === 'indeterminate' ? false : e);
  };

  return (
    <div className="editor-container">
      {(initialValue.key === 'json' || initialValue.key === 'chunks') && (
        <ArrayContainer
          isReadonly={isReadonly}
          className={className}
          initialValue={initialValue}
          handleCheck={handleCheck}
          selectedChunkIds={selectedChunkIds}
          onSave={onSave}
          textMode={textMode}
          isChunck={isChunck}
          clickChunk={clickChunk}
          newChunkIndex={newChunkIndex}
        />
      )}

      {(initialValue.key === 'text' ||
        initialValue.key === 'html' ||
        initialValue.key === 'markdown') && (
        <ObjectContainer
          isReadonly={isReadonly}
          className={className}
          initialValue={initialValue}
          onSave={onSave}
          textMode={textMode}
          isChunck={isChunck}
          clickChunk={clickChunk}
        />
      )}
    </div>
  );
};

export default FormatPreserveEditor;
