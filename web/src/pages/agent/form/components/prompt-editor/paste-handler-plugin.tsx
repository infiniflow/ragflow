import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import {
  $createParagraphNode,
  $createTextNode,
  $getSelection,
  $isRangeSelection,
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

        // Check if text contains line breaks
        if (text.includes('\n')) {
          editor.update(() => {
            const selection = $getSelection();
            if (selection && $isRangeSelection(selection)) {
              // Normalize line breaks, merge multiple consecutive line breaks into a single line break
              const normalizedText = text.replace(/\n{2,}/g, '\n');

              // Clear current selection
              selection.removeText();

              // Create a paragraph node to contain all content
              const paragraph = $createParagraphNode();

              // Split text by line breaks
              const lines = normalizedText.split('\n');

              // Process each line
              lines.forEach((lineText, index) => {
                // Add line text (if any)
                if (lineText) {
                  const textNode = $createTextNode(lineText);
                  paragraph.append(textNode);
                }

                // If not the last line, add a line break
                if (index < lines.length - 1) {
                  const lineBreak = $createTextNode('\n');
                  paragraph.append(lineBreak);
                }
              });

              // Insert paragraph
              selection.insertNodes([paragraph]);
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
