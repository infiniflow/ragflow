import {
  $getRoot,
  $getSelection,
  $isRangeSelection,
  LexicalEditor,
} from 'lexical';

export function insertVariableTrigger(editor: LexicalEditor) {
  editor.update(
    () => {
      const root = $getRoot();
      const trigger = root.getTextContent() === '' ? '/' : ' /';
      const selection = $getSelection();
      if ($isRangeSelection(selection)) {
        selection.insertText(trigger);
      } else {
        root.selectEnd().insertText(trigger);
      }
    },
    { discrete: true },
  );
  editor.focus();
}
