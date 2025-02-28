/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 *
 */

import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import {
  LexicalTypeaheadMenuPlugin,
  MenuOption,
  useBasicTypeaheadTriggerMatch,
} from '@lexical/react/LexicalTypeaheadMenuPlugin';
import {
  $createParagraphNode,
  $createTextNode,
  $getRoot,
  $getSelection,
  $isRangeSelection,
  TextNode,
} from 'lexical';
import {
  ReactElement,
  useCallback,
  useContext,
  useEffect,
  useRef,
} from 'react';
import * as ReactDOM from 'react-dom';

import { FlowFormContext } from '@/pages/flow/context';
import { useBuildComponentIdSelectOptions } from '@/pages/flow/hooks/use-get-begin-query';
import { $createVariableNode } from './variable-node';

import { ProgrammaticTag } from './constant';
import './index.css';
class VariableInnerOption extends MenuOption {
  label: string;
  value: string;

  constructor(label: string, value: string) {
    super(value);
    this.label = label;
    this.value = value;
  }
}

class VariableOption extends MenuOption {
  label: ReactElement | string;
  title: string;
  options: VariableInnerOption[];

  constructor(
    label: ReactElement | string,
    title: string,
    options: VariableInnerOption[],
  ) {
    super(title);
    this.label = label;
    this.title = title;
    this.options = options;
  }
}

function VariablePickerMenuItem({
  index,
  option,
  selectOptionAndCleanUp,
}: {
  index: number;
  option: VariableOption;
  selectOptionAndCleanUp: (
    option: VariableOption | VariableInnerOption,
  ) => void;
}) {
  return (
    <li
      key={option.key}
      tabIndex={-1}
      ref={option.setRefElement}
      role="option"
      id={'typeahead-item-' + index}
    >
      <div>
        <span className="text text-slate-500">{option.title}</span>
        <ul className="pl-2 py-1">
          {option.options.map((x) => (
            <li
              key={x.value}
              onClick={() => selectOptionAndCleanUp(x)}
              className="hover:bg-slate-300 p-1"
            >
              {x.label}
            </li>
          ))}
        </ul>
      </div>
    </li>
  );
}

export default function VariablePickerMenuPlugin({
  value,
}: {
  value?: string;
}): JSX.Element {
  const [editor] = useLexicalComposerContext();
  const isFirstRender = useRef(true);

  const node = useContext(FlowFormContext);

  const checkForTriggerMatch = useBasicTypeaheadTriggerMatch('/', {
    minLength: 0,
  });

  const setQueryString = useCallback(() => {}, []);

  const options = useBuildComponentIdSelectOptions(node?.id, node?.parentId);

  const nextOptions: VariableOption[] = options.map(
    (x) =>
      new VariableOption(
        x.label,
        x.title,
        x.options.map((y) => new VariableInnerOption(y.label, y.value)),
      ),
  );

  const findLabelByValue = useCallback(
    (value: string) => {
      const children = options.reduce<Array<{ label: string; value: string }>>(
        (pre, cur) => {
          return pre.concat(cur.options);
        },
        [],
      );

      return children.find((x) => x.value === value)?.label;
    },
    [options],
  );

  const onSelectOption = useCallback(
    (
      selectedOption: VariableOption | VariableInnerOption,
      nodeToRemove: TextNode | null,
      closeMenu: () => void,
    ) => {
      editor.update(() => {
        const selection = $getSelection();

        if (!$isRangeSelection(selection) || selectedOption === null) {
          return;
        }

        if (nodeToRemove) {
          nodeToRemove.remove();
        }

        selection.insertNodes([
          $createVariableNode(
            (selectedOption as VariableInnerOption).value,
            selectedOption.label as string,
          ),
        ]);

        closeMenu();
      });
    },
    [editor],
  );

  const parseTextToVariableNodes = useCallback(
    (text: string) => {
      const paragraph = $createParagraphNode();

      // Regular expression to match content within {}
      const regex = /{([^}]*)}/g;
      let match;
      let lastIndex = 0;

      while ((match = regex.exec(text)) !== null) {
        const { 1: content, index, 0: template } = match;

        // Add the previous text part (if any)
        if (index > lastIndex) {
          const textNode = $createTextNode(text.slice(lastIndex, index));

          paragraph.append(textNode);
        }

        // Add variable node or text node
        const label = findLabelByValue(content);
        if (label) {
          paragraph.append($createVariableNode(content, label));
        } else {
          paragraph.append($createTextNode(template));
        }

        // Update index
        lastIndex = regex.lastIndex;
      }

      // Add the last part of text (if any)
      if (lastIndex < text.length) {
        const textNode = $createTextNode(text.slice(lastIndex));
        paragraph.append(textNode);
      }

      $getRoot().clear().append(paragraph);
    },
    [findLabelByValue],
  );

  useEffect(() => {
    if (editor && value && isFirstRender.current) {
      isFirstRender.current = false;
      editor.update(
        () => {
          parseTextToVariableNodes(value);
        },
        { tag: ProgrammaticTag },
      );
    }
  }, [parseTextToVariableNodes, editor, value]);

  return (
    <LexicalTypeaheadMenuPlugin<VariableOption | VariableInnerOption>
      onQueryChange={setQueryString}
      onSelectOption={onSelectOption}
      triggerFn={checkForTriggerMatch}
      options={nextOptions}
      menuRenderFn={(anchorElementRef, { selectOptionAndCleanUp }) =>
        anchorElementRef.current && options.length
          ? ReactDOM.createPortal(
              <div className="typeahead-popover w-[200px] p-2">
                <ul>
                  {nextOptions.map((option, i: number) => (
                    <VariablePickerMenuItem
                      index={i}
                      key={option.key}
                      option={option}
                      selectOptionAndCleanUp={selectOptionAndCleanUp}
                    />
                  ))}
                </ul>
              </div>,
              anchorElementRef.current,
            )
          : null
      }
    />
  );
}
