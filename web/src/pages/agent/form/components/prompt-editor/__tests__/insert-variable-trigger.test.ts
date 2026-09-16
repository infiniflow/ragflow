import {
  $createParagraphNode,
  $createTextNode,
  $getRoot,
  createEditor,
  LexicalEditor,
} from 'lexical';
import { insertVariableTrigger } from '../insert-variable-trigger';

describe('insertVariableTrigger', () => {
  let editor: LexicalEditor;
  let rootElement: HTMLDivElement;

  beforeEach(() => {
    editor = createEditor({
      namespace: 'insert-variable-trigger-test',
      onError: (error) => {
        throw error;
      },
    });
    rootElement = document.createElement('div');
    document.body.appendChild(rootElement);
    editor.setRootElement(rootElement);
  });

  afterEach(() => {
    editor.setRootElement(null);
    rootElement.remove();
  });

  it('inserts the variable menu trigger when the editor has never been focused', () => {
    insertVariableTrigger(editor);

    editor.getEditorState().read(() => {
      expect($getRoot().getTextContent()).toBe(' /');
    });
  });

  it('inserts the variable menu trigger at the existing caret', () => {
    editor.update(
      () => {
        const text = $createTextNode('hello');
        $getRoot().append($createParagraphNode().append(text));
        text.select(2, 2);
      },
      { discrete: true },
    );

    insertVariableTrigger(editor);

    editor.getEditorState().read(() => {
      expect($getRoot().getTextContent()).toBe('he /llo');
    });
  });
});