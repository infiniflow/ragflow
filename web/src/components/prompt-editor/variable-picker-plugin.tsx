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
  useState,
} from 'react';
import * as ReactDOM from 'react-dom';

import { FlowFormContext } from '@/pages/flow/context';
import { useBuildComponentIdSelectOptions } from '@/pages/flow/hooks/use-get-begin-query';
import { $createVariableNode } from './variable-node';

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
  selectOptionAndCleanUp: (option: VariableOption) => void;
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
  const [queryString, setQueryString] = useState<string | null>(null);
  const isFirstRender = useRef(true);

  const node = useContext(FlowFormContext);

  const checkForTriggerMatch = useBasicTypeaheadTriggerMatch('/', {
    minLength: 0,
  });

  const options = useBuildComponentIdSelectOptions(node?.id, node?.parentId);

  const nextOptions: VariableOption[] = options.map(
    (x) =>
      new VariableOption(
        x.label,
        x.title,
        x.options.map((y) => new VariableInnerOption(y.label, y.value)),
      ),
  );

  const onSelectOption = useCallback(
    (
      selectedOption: VariableOption,
      nodeToRemove: TextNode | null,
      closeMenu: () => void,
    ) => {
      console.log(
        '🚀 ~ VariablePickerMenuPlugin ~ selectedOption:',
        selectedOption,
      );
      editor.update(() => {
        const selection = $getSelection();

        if (!$isRangeSelection(selection) || selectedOption === null) {
          return;
        }

        if (nodeToRemove) {
          nodeToRemove.remove();
        }

        selection.insertNodes([
          $createVariableNode(selectedOption.value, selectedOption.label),
        ]);

        closeMenu();
      });
    },
    [editor],
  );

  useEffect(() => {
    if (editor && value && isFirstRender.current) {
      isFirstRender.current = false;
      editor.update(() => {
        const paragraph = $createParagraphNode();
        const textNode = $createTextNode(value);

        paragraph.append(textNode);

        $getRoot().clear().append(paragraph);
      });
    }
  }, [editor, value]);

  return (
    <>
      <LexicalTypeaheadMenuPlugin<VariableOption>
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
    </>
  );
}
