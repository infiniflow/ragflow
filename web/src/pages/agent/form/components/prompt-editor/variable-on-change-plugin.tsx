import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import { EditorState, LexicalEditor, TextNode } from 'lexical';
import { useEffect } from 'react';
import { ProgrammaticTag } from './constant';
import { mergeLeadingVariablePathTextNode } from './variable-path-transform';

interface VariableOnChangePluginProps {
  enablePathQueryAutoMerge?: boolean;
  onChange: (
    editorState: EditorState,
    editor?: LexicalEditor,
    tags?: Set<string>,
  ) => void;
}

export function VariableOnChangePlugin({
  enablePathQueryAutoMerge = true,
  onChange,
}: VariableOnChangePluginProps) {
  // Access the editor through the LexicalComposerContext
  const [editor] = useLexicalComposerContext();
  // Wrap our listener in useEffect to handle the teardown and avoid stale references.
  useEffect(() => {
    const removeTransform = enablePathQueryAutoMerge
      ? editor.registerNodeTransform(TextNode, mergeLeadingVariablePathTextNode)
      : () => {};
    const removeUpdateListener = editor.registerUpdateListener(
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

    return () => {
      removeTransform();
      removeUpdateListener();
    };
  }, [editor, enablePathQueryAutoMerge, onChange]);

  return null;
}
