import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import {
  $createLineBreakNode,
  $createTextNode,
  $getSelection,
  $isRangeSelection,
  LexicalNode,
  PASTE_COMMAND,
} from 'lexical';
import { useEffect } from 'react';

function PasteHandlerPlugin() {
  const [editor] = useLexicalComposerContext();

  useEffect(() => {
    const removeListener = editor.registerCommand(
      PASTE_COMMAND,
      (clipboardEvent: ClipboardEvent) => {
        const clipboardData = clipboardEvent.clipboardData;
        if (!clipboardData) {
          return false;
        }

        const text = clipboardData.getData('text/plain');
        if (!text) {
          return false;
        }

        // Handle text with line breaks
        if (text.includes('\n')) {
          editor.update(() => {
            const selection = $getSelection();
            if (selection && $isRangeSelection(selection)) {
              // Build an array of nodes (TextNodes and LineBreakNodes).
              // Insert nodes directly into selection to avoid creating
              // extra paragraph boundaries which cause newline multiplication.
              const nodes: LexicalNode[] = [];
              const lines = text.split('\n');

              lines.forEach((lineText, index) => {
                if (lineText) {
                  nodes.push($createTextNode(lineText));
                }

                // Add LineBreakNode between lines (not after the last line)
                if (index < lines.length - 1) {
                  nodes.push($createLineBreakNode());
                }
              });

              selection.insertNodes(nodes);
            }
          });

          // Prevent default paste behavior
          clipboardEvent.preventDefault();
          return true;
        }

        // If no line breaks, use default behavior
        return false;
      },
      4,
    );

    return () => {
      removeListener();
    };
  }, [editor]);

  return null;
}

export { PasteHandlerPlugin };
