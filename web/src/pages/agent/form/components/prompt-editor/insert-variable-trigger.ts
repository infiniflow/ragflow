import { $getRoot, $getSelection, $isRangeSelection, LexicalEditor } from 'lexical';

export function insertVariableTrigger(editor: LexicalEditor) {
  editor.update(
    () => {
      const selection = $getSelection();
      if ($isRangeSelection(selection)) {
        selection.insertText(' /');
      } else {
        $getRoot().selectEnd().insertText(' /');
      }
    },
    { discrete: true },
  );
  editor.focus();
}