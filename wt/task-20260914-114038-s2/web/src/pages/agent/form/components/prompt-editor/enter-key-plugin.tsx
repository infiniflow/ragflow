import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import {
  $createLineBreakNode,
  $getSelection,
  $isRangeSelection,
  COMMAND_PRIORITY_HIGH,
  KEY_ENTER_COMMAND,
} from 'lexical';
import { useEffect } from 'react';

// This plugin overrides the default Enter key behavior.
// Instead of creating a new paragraph (which adds \n\n in getTextContent),
// it creates a LineBreakNode (which adds \n in getTextContent).
// This ensures consistent serialization between typed and pasted content.
function EnterKeyPlugin() {
  const [editor] = useLexicalComposerContext();

  useEffect(() => {
    const removeListener = editor.registerCommand(
      KEY_ENTER_COMMAND,
      (event: KeyboardEvent | null) => {
        // Allow Shift+Enter to use default behavior (if needed for other purposes)
        if (event?.shiftKey) {
          return false;
        }

        const selection = $getSelection();
        if (selection && $isRangeSelection(selection)) {
          // Prevent default paragraph creation
          event?.preventDefault();

          // Insert a LineBreakNode at cursor position
          selection.insertNodes([$createLineBreakNode()]);
          return true;
        }

        return false;
      },
      COMMAND_PRIORITY_HIGH,
    );

    return () => {
      removeListener();
    };
  }, [editor]);

  return null;
}

export { EnterKeyPlugin };
