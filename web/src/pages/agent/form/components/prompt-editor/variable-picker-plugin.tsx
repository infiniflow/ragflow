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
} from '@lexical/react/LexicalTypeaheadMenuPlugin';
import {
  $createLineBreakNode,
  $createParagraphNode,
  $createTextNode,
  $getRoot,
  $getSelection,
  $isRangeSelection,
  COMMAND_PRIORITY_CRITICAL,
  TextNode,
} from 'lexical';
import React, {
  createContext,
  ForwardedRef,
  forwardRef,
  HTMLAttributes,
  ReactElement,
  ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
} from 'react';
import * as ReactDOM from 'react-dom';

import { $createVariableNode } from './variable-node';

import { ScrollArea } from '@/components/ui/scroll-area';
import { cn } from '@/lib/utils';
import { JsonSchemaDataType, VariableRegex } from '@/pages/agent/constant';
import {
  useFindAgentStructuredOutputLabel,
  useGetStructuredOutputByValue,
  useShowSecondaryMenu,
} from '@/pages/agent/hooks/use-build-structured-output';
import { useFilterQueryVariableOptionsByTypes } from '@/pages/agent/hooks/use-get-begin-query';
import { LucideChevronRight } from 'lucide-react';
import { PromptIdentity } from '../../agent-form/use-build-prompt-options';
import { StructuredOutputSecondaryMenu } from '../structured-output-secondary-menu';
import { ProgrammaticTag } from './constant';
import './index.css';

const SelectedValueContext = createContext<string>('');

class VariableOption extends MenuOption {
  label: string;
  value: string;
  parentLabel: string | JSX.Element;
  icon?: ReactNode;
  type?: string;
  options?: VariableOption[];

  constructor(
    label: string,
    value: string,
    parentLabel: string | JSX.Element,
    icon?: ReactNode,
    type?: string,
  ) {
    super(value);
    this.label = label;
    this.value = value;
    this.parentLabel = parentLabel;
    this.icon = icon;
    this.type = type;
  }
}

class VariableOptionGroup extends MenuOption {
  label: ReactElement | string;
  title: string;
  options: VariableOption[];

  constructor(
    label: ReactElement | string,
    title: string,
    options: VariableOption[],
  ) {
    super(title);
    this.label = label;
    this.title = title;
    this.options = options;
  }
}

const VariablePickerOption = forwardRef(function VariablePickerOption(
  {
    option,
    label,
    hasSubMenu = false,
    className,
    onClick,
    ...props
  }: {
    option: VariableOption;
    label?: string;
    hasSubMenu?: boolean;
    className?: string;
    onClick?: () => void;
  } & HTMLAttributes<HTMLLIElement>,
  ref: ForwardedRef<HTMLElement>,
) {
  const selectedValue = useContext(SelectedValueContext);
  const isSelected = option.value === selectedValue;

  return (
    <li
      {...props}
      ref={ref as ForwardedRef<HTMLLIElement>}
      key={option.key}
      onClick={onClick}
      className={cn(
        'px-2 py-1 text-text-primary rounded-sm flex justify-between items-center',
        'hover:bg-bg-card focus-visible:bg-bg-card',
        isSelected && 'bg-bg-card',
        className,
      )}
      role="option"
      aria-label={option.label}
      aria-selected={isSelected}
    >
      <span className="truncate flex-1 min-w-0">{label ?? option.label}</span>
      <span className="text-text-secondary text-xs">{option.type}</span>
      {hasSubMenu ? (
        <LucideChevronRight className="ml-2 size-[1em] text-text-secondary" />
      ) : null}
    </li>
  );
});

// TODO: Stage 2
/*
function VariableStructuredOptions({
  option,
  children,
  selectOptionAndCleanUp,
}: {
  option: VariableOption;
  children?: ReactNode;
  selectOptionAndCleanUp: (option: VariableOption) => void;
}) {
  const selectedValue = useContext(SelectedValueContext);

  const hasSelectedDescendant = useMemo(() => {
    const _hasSelectedDescendant = (options: VariableOption[]): boolean => {
      let result = false;

      for (const x of options) {
        if (x.value === selectedValue) {
          return true;
        }

        if (x.options?.length) {
          result = result || _hasSelectedDescendant(x.options);
        }
      }

      return result;
    };

    return _hasSelectedDescendant(option?.options ?? []);
  }, [option?.options, selectedValue]);

  const renderStructuredOptions = useCallback((options?: VariableOption[], level = 0) => {
    if (!options?.length) {
      return null;
    }

    return (
      <ul
        className={cn(
          level > 0 && 'border-l !ml-2',
        )}
      >
        {options.map((o) => {
          if (o.options?.length) {
            return (
              <div key={o.key}>
                <VariablePickerOption
                  key={o.key}
                  ref={o.setRefElement}
                  option={o}
                  label={o.label.split('.').pop()}
                  className={o.value === selectedValue ? 'bg-bg-card' : ''}
                  onClick={() => selectOptionAndCleanUp(o)}
                />

                {renderStructuredOptions(o.options, level + 1)}
              </div>
            );
          }

          return (
            <VariablePickerOption
              key={o.key}
              ref={o.setRefElement}
              option={o}
              label={o.label.split('.').pop()}
              onClick={() => selectOptionAndCleanUp(o)}
            />
          );
        })}
      </ul>
    );
  }, []);

  return (
    <HoverCard
      open={hasSelectedDescendant || undefined}
      openDelay={100}
      closeDelay={100}
    >
      <HoverCardTrigger asChild>
        <VariablePickerOption
          key={option?.key}
          ref={option?.setRefElement}
          option={option}
          onClick={() => selectOptionAndCleanUp(option)}
          hasSubMenu
        />
      </HoverCardTrigger>

      <HoverCardContent
        side="left"
        align="start"
        className="bg-bg-base min-w-72 border-0.5 border-border rounded-md shadow-lg p-0"
      >
        <ScrollArea className="p-2">
          <section>
            <header className="mb-2 text-text-secondary text-xs">
              {t('flow.structuredOutput.structuredOutput')}
            </header>

            {renderStructuredOptions(option?.options)}
          </section>
        </ScrollArea>
      </HoverCardContent>
    </HoverCard>
  );
}
*/

function VariablePickerOptionGroup({
  title,
  options = [],
  selectOptionAndCleanUp,
  types,
}: {
  title?: string;
  options?: VariableOption[];
  types?: JsonSchemaDataType[];
  selectOptionAndCleanUp: (option: VariableOption) => void;
}) {
  const showSecondaryMenu = useShowSecondaryMenu();
  const selectedValue = useContext(SelectedValueContext);

  return (
    <ul role="group" aria-label={title}>
      <li key={title} role="presentation">
        <div className="text-xs text-text-secondary cursor-auto py-1">
          {title}
        </div>
      </li>

      {options.map((x) => {
        const shouldShowSecondary = showSecondaryMenu(x.value, x.label);
        const isSelected = x.value === selectedValue;

        if (shouldShowSecondary) {
          // TODO: Stage 2
          /*
          if (
            !isEmpty(types)
            && !hasSpecificTypeChild(x ?? {}, types)
            && !types?.some((x) => x === JsonSchemaDataType.Object)
          ) {
            return null;
          }

          return (
            <VariableStructuredOptions
              key={`subMenu:${x.key}`}
              option={x}
              selectOptionAndCleanUp={selectOptionAndCleanUp}
            />
          );
          */

          return (
            <StructuredOutputSecondaryMenu
              ref={x.setRefElement}
              key={x.value}
              data={x}
              types={types}
              className={isSelected ? 'bg-bg-card' : ''}
              click={(y) =>
                selectOptionAndCleanUp({
                  ...x,
                  ...y,
                } as VariableOption)
              }
            />
          );
        }

        return (
          <VariablePickerOption
            key={x.key}
            ref={x.setRefElement}
            option={x}
            onClick={() => selectOptionAndCleanUp(x)}
          />
        );
      })}
    </ul>
  );
}

export type VariablePickerMenuOptionType = {
  label: string;
  title: string;
  value?: string;
  options: Array<{
    label: string;
    value: string;
    icon: ReactNode;
    type?: string;
  }>;
};

export type VariablePickerMenuPluginProps = {
  value?: string;
  extraOptions?: VariablePickerMenuOptionType[];
  baseOptions?: VariablePickerMenuOptionType[];
  types?: JsonSchemaDataType[];
};

export default function VariablePickerMenuPlugin({
  value,
  extraOptions,
  baseOptions,
  types,
}: VariablePickerMenuPluginProps): JSX.Element {
  const [editor] = useLexicalComposerContext();
  // const shouldShowSecondaryMenu = useShowSecondaryMenu();
  const findAgentStructuredOutputLabel = useFindAgentStructuredOutputLabel();
  const filterStructuredOutput = useGetStructuredOutputByValue();

  const testTriggerFn = React.useCallback((text: string) => {
    const triggerRegex = /(^|\s|\()([/]((?:[^/\s\()])*))$/;
    const match = triggerRegex.exec(text);

    if (match !== null) {
      const mayLeadingWhitespace = match[1];

      return {
        leadOffset: match.index + mayLeadingWhitespace.length,
        matchingString: match[3], // This will send to onQueryChange() event handler
        replaceableString: match[2],
      };
    }

    return null;
  }, []);

  const previousValue = useRef<string | undefined>();
  const [queryString, setQueryString] = React.useState<string>('');

  let options = useFilterQueryVariableOptionsByTypes({ types });

  if (baseOptions) {
    options = baseOptions as typeof options;
  }

  const unifiedOptions = useMemo(() => {
    const allGroups = Array.from(
      [...options, ...(extraOptions ?? [])],
      (g) =>
        new VariableOptionGroup(
          g.label,
          g.title,
          g.options as VariableOption[],
        ),
    );

    // TODO: Stage 2
    /*
    const _treeify = (values: any, option: VariableOption): void | VariableOption[] => {
      if (values == null) {
        return;
      }

      const properties = get(values, 'properties') || get(values, 'items.properties');

      if (isPlainObject(values) && properties) {
        option.options = Object.entries(properties).map(([key, value]) => {
          const nextOption = new VariableOption(
            `${option.label}.${key}`,
            `${option.value}.${key}`,
            option.label,
          );

          const {
            dataType,
            compositeDataType,
          } = getStructuredDatatype(value);

          if (
            isEmpty(types)
            || types?.some((x) => x === compositeDataType)
            || hasSpecificTypeChild(value ?? {}, types)
          ) {

            nextOption.type = compositeDataType;

            if ([JsonSchemaDataType.Object, JsonSchemaDataType.Array].some(x => x === dataType)) {
              _treeify(value, nextOption)!;
            }
          }

          return nextOption;
        });
      }
    };
    */

    const treeified = allGroups.map((group) => {
      group.options = group.options.map((option) => {
        const newOption = new VariableOption(
          option.label,
          option.value,
          option.parentLabel,
          option.icon,
          option.type,
        );

        // TODO: Stage 2
        /*
        if (shouldShowSecondaryMenu(newOption.value, newOption.label)) {
          const structuredOutput = _treeify(filterStructuredOutput(newOption.value), newOption);

          if (structuredOutput) {
            newOption.options = structuredOutput;
          }
        }
        */

        return newOption;
      });

      return group;
    });

    const filtered = treeified
      .map((g) => ({
        ...g,
        options: g.options.filter((y) => {
          if (!queryString) {
            return true;
          }

          // TODO: Stage 2
          // Stage 1: Allow filtering by component label, such as: "agent_0.content", "retrieval_0.json", etc.
          const parentLabel =
            typeof y.parentLabel === 'string'
              ? `${y.parentLabel.toLowerCase()}.`
              : '';
          const thisLabel = `${parentLabel}${y.label.toLowerCase()}`;
          const thisValue = `${parentLabel}${y.value.toLowerCase()}`;

          return (
            thisLabel.includes(queryString) || thisValue.includes(queryString)
          );
        }),
      }))
      .filter((x) => x.options.length);

    const _flat = (
      option: VariableOption,
    ): VariableOption | VariableOption[] => {
      if (option.options) {
        return [option, ...option.options.flatMap((x) => _flat(x))];
      }

      return option;
    };

    const flattened: VariableOption[] = filtered
      .flatMap((x) => x?.options ?? [])
      .flatMap((x) => _flat(x));

    return {
      treeified: filtered,
      flattened,
    };
  }, [queryString, options, extraOptions, filterStructuredOutput]);

  const findItemByValue = useCallback(
    (value: string) => {
      const children = options.reduce<
        Array<{
          label: string;
          value: string;
          parentLabel?: string | ReactNode;
          icon?: ReactNode;
        }>
      >((pre, cur) => {
        return pre.concat(cur.options);
      }, []);

      // agent structured output
      const agentStructuredOutput = findAgentStructuredOutputLabel(
        value,
        children,
      );
      if (agentStructuredOutput) {
        return agentStructuredOutput;
      }

      return children.find((x) => x.value === value);
    },
    [findAgentStructuredOutputLabel, options],
  );

  const onSelectOption = useCallback(
    (
      selectedOption: VariableOption,
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
        const variableNode = $createVariableNode(
          (selectedOption as VariableOption).value,
          selectedOption.label as string,
          selectedOption.parentLabel as string | ReactNode,
          selectedOption.icon as ReactNode,
        );
        if (selectedOption.parentLabel === PromptIdentity) {
          selection.insertText(selectedOption.value);
        } else {
          selection.insertNodes([variableNode]);
        }

        closeMenu();
      });
    },
    [editor],
  );

  // Parses a single line of text and appends nodes to the paragraph.
  // Handles variable references in the format {variable_name}.
  const parseLineContent = useCallback(
    (line: string, paragraph: ReturnType<typeof $createParagraphNode>) => {
      const regex = VariableRegex;
      let match;
      let lastIndex = 0;

      while ((match = regex.exec(line)) !== null) {
        const { 1: content, index, 0: template } = match;

        if (index > lastIndex) {
          paragraph.append($createTextNode(line.slice(lastIndex, index)));
        }

        const nodeItem = findItemByValue(content);
        if (nodeItem) {
          paragraph.append(
            $createVariableNode(
              content,
              nodeItem.label,
              nodeItem.parentLabel,
              nodeItem.icon,
            ),
          );
        } else {
          paragraph.append($createTextNode(template));
        }

        lastIndex = regex.lastIndex;
      }

      if (lastIndex < line.length) {
        paragraph.append($createTextNode(line.slice(lastIndex)));
      }
    },
    [findItemByValue],
  );

  // Parses text content into a single paragraph with LineBreakNodes for newlines.
  // Using LineBreakNode ensures proper rendering and consistent serialization.
  const parseTextToVariableNodes = useCallback(
    (text: string) => {
      const paragraph = $createParagraphNode();
      const lines = text.split('\n');

      lines.forEach((line, index) => {
        // Parse the line content (text and variables)
        if (line) {
          parseLineContent(line, paragraph);
        }

        // Add LineBreakNode between lines (not after the last line)
        if (index < lines.length - 1) {
          paragraph.append($createLineBreakNode());
        }
      });

      $getRoot().clear().append(paragraph);

      if ($isRangeSelection($getSelection())) {
        $getRoot().selectEnd();
      }
    },
    [parseLineContent],
  );

  useEffect(() => {
    if (editor && value && value !== previousValue.current) {
      previousValue.current = value;
      editor.update(
        () => {
          parseTextToVariableNodes(value);
        },
        { tag: ProgrammaticTag },
      );
    }
  }, [parseTextToVariableNodes, editor, value]);

  // Fixed the issue where the cursor would go to the end when changing its own data
  useEffect(() => {
    return editor.registerUpdateListener(({ editorState, tags }) => {
      // If we trigger the programmatic update ourselves, we should not write back to avoid an infinite loop.
      if (tags.has(ProgrammaticTag)) return;

      editorState.read(() => {
        const text = $getRoot().getTextContent();
        if (text !== previousValue.current) {
          previousValue.current = text;
        }
      });
    });
  }, [editor]);

  return (
    <LexicalTypeaheadMenuPlugin
      // Lift the priority to the highest level to prevent the default Enter key behavior from being triggered
      commandPriority={COMMAND_PRIORITY_CRITICAL}
      onQueryChange={(s) => setQueryString(s?.toLowerCase() ?? '')}
      onSelectOption={(option, textNodeContainingQuery, closeMenu) =>
        onSelectOption(
          option as VariableOption, // Only the second level menu can be selected
          textNodeContainingQuery,
          closeMenu,
        )
      }
      triggerFn={testTriggerFn}
      options={unifiedOptions.flattened}
      menuRenderFn={(
        anchorElementRef,
        { selectOptionAndCleanUp, options, selectedIndex },
      ) => {
        if (!anchorElementRef.current || !unifiedOptions.flattened.length) {
          return null;
        }

        return ReactDOM.createPortal(
          <SelectedValueContext.Provider
            value={
              (selectedIndex !== null &&
                (options[selectedIndex] as VariableOption)?.value) ||
              ''
            }
          >
            <div className="typeahead-popover w-80 bg-bg-base border-0.5 border-border">
              <ScrollArea className="p-2">
                <div className="max-h-64 space-y-2">
                  {unifiedOptions.treeified.map((group) => (
                    <VariablePickerOptionGroup
                      key={group.title}
                      title={group.title}
                      options={group.options}
                      types={types}
                      selectOptionAndCleanUp={selectOptionAndCleanUp}
                    />
                  ))}
                </div>
              </ScrollArea>
            </div>
          </SelectedValueContext.Provider>,
          anchorElementRef.current,
        );
      }}
    />
  );
}
