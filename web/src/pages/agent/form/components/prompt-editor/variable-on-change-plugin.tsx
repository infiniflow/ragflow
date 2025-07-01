import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import { EditorState, LexicalEditor } from 'lexical';
import { useEffect } from 'react';
import { ProgrammaticTag } from './constant';

interface IProps {
  onChange: (
    editorState: EditorState,
    editor?: LexicalEditor,
    tags?: Set<string>,
  ) => void;
}

export function VariableOnChangePlugin({ onChange }: IProps) {
  // Access the editor through the LexicalComposerContext
  const [editor] = useLexicalComposerContext();
  // Wrap our listener in useEffect to handle the teardown and avoid stale references.
  useEffect(() => {
    // most listeners return a teardown function that can be called to clean them up.
    return editor.registerUpdateListener(
      ({ editorState, tags, dirtyElements }) => {
        // Check if there is a "programmatic" tag
        const isProgrammaticUpdate = tags.has(ProgrammaticTag);

        // The onchange event is only triggered when the data is manually updated
        // Otherwise, the content will be displayed incorrectly.
        if (dirtyElements.size > 0 && !isProgrammaticUpdate) {
          onChange(editorState);
        }
      },
    );
  }, [editor, onChange]);

  return null;
}
