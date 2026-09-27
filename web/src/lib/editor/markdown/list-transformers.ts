/**
 * List transformers for markdown import/export.
 * Ported from Nimbalyst with minimal dependencies.
 */

import type { ListType } from '@lexical/list';
import {
  $createListItemNode,
  $createListNode,
  $isListItemNode,
  $isListNode,
  ListItemNode,
  ListNode,
} from '@lexical/list';
import type { ElementTransformer } from '@lexical/markdown';
import type { ElementNode, LexicalNode } from 'lexical';

export const ORDERED_LIST_REGEX = /^(\s*)(\d{1,})\.\s/;
export const UNORDERED_LIST_REGEX = /^(\s*)[-*+]\s/;
export const CHECK_LIST_REGEX = /^(\s*)(?:-\s)?\s?(\[(\s|x)?\])\s/i;

export interface ListConfig {
  exportIndentSize?: number;
  importMinIndentSize?: number;
  importMaxIndentSize?: number;
  autoDetectIndent?: boolean;
}

const DEFAULT_LIST_CONFIG: ListConfig = {
  exportIndentSize: 2,
  importMinIndentSize: 2,
  importMaxIndentSize: 4,
  autoDetectIndent: true,
};

let globalListConfig: ListConfig = { ...DEFAULT_LIST_CONFIG };

export function setListConfig(config: Partial<ListConfig>): void {
  globalListConfig = { ...DEFAULT_LIST_CONFIG, ...config };
}

export function getListConfig(): ListConfig {
  return { ...globalListConfig };
}

export function getIndentLevel(
  whitespaces: string,
  isImport: boolean = true,
): number {
  const tabs = whitespaces.match(/\t/g);
  const spaces = whitespaces.match(/ /g);
  let indent = 0;
  if (tabs) indent += tabs.length;
  if (spaces) {
    const spaceCount = spaces.length;
    const config = getListConfig();
    if (isImport) {
      const importSize = config.importMinIndentSize ?? 2;
      indent += Math.floor(spaceCount / importSize);
    } else {
      const exportSize = config.exportIndentSize ?? 2;
      indent += Math.floor(spaceCount / exportSize);
    }
  }
  return indent;
}

export function createListReplace(
  listType: ListType,
): ElementTransformer['replace'] {
  return (parentNode, children, match, isImport) => {
    const previousNode = parentNode.getPreviousSibling();
    const nextNode = parentNode.getNextSibling();
    const listItem = $createListItemNode(
      listType === 'check' ? match[3] === 'x' : undefined,
    );
    const indent = getIndentLevel(match[1], true);

    if ($isListNode(nextNode) && nextNode.getListType() === listType) {
      const firstChild = nextNode.getFirstChild();
      if (firstChild !== null) firstChild.insertBefore(listItem);
      else nextNode.append(listItem);
      parentNode.remove();
    } else if (
      $isListNode(previousNode) &&
      previousNode.getListType() === listType
    ) {
      previousNode.append(listItem);
      parentNode.remove();
    } else {
      const list = $createListNode(
        listType,
        listType === 'number' ? Number(match[2]) : undefined,
      );
      list.append(listItem);
      parentNode.replace(list);
    }

    listItem.append(...children);
    if (!isImport) listItem.select(0, 0);
    if (indent > 0) listItem.setIndent(indent);
  };
}

export function listExport(
  listNode: ListNode,
  exportChildren: (node: ElementNode) => string,
  depth: number = 0,
  config?: ListConfig,
): string {
  const mergedConfig = { ...globalListConfig, ...config };
  const indentSize = mergedConfig.exportIndentSize ?? 2;
  const output: string[] = [];
  const children = listNode.getChildren();
  let index = 0;

  for (const listItemNode of children) {
    if (!$isListItemNode(listItemNode)) continue;

    if (listItemNode.getChildrenSize() === 1) {
      const firstChild = listItemNode.getFirstChild();
      if ($isListNode(firstChild)) {
        output.push(listExport(firstChild, exportChildren, depth + 1, config));
        continue;
      }
    }

    const itemIndent = listItemNode.getIndent();
    const actualDepth = itemIndent !== undefined ? itemIndent : depth;
    const indent = ' '.repeat(actualDepth * indentSize);
    const listType = listNode.getListType();
    const prefix =
      listType === 'number'
        ? `${listNode.getStart() + index}. `
        : listType === 'check'
          ? `- [${listItemNode.getChecked() ? 'x' : ' '}] `
          : '- ';

    output.push(indent + prefix + exportChildren(listItemNode));
    index++;
  }
  return output.join('\n');
}

export function createListTransformer(
  listType: ListType,
  regex: RegExp,
): ElementTransformer {
  return {
    dependencies: [ListNode, ListItemNode],
    export: (
      node: LexicalNode,
      exportChildren: (node: ElementNode) => string,
    ) => {
      if (!$isListNode(node)) return null;
      return listExport(node, exportChildren, 0, getListConfig());
    },
    regExp: regex,
    replace: createListReplace(listType),
    type: 'element',
  };
}

export const UNORDERED_LIST: ElementTransformer = createListTransformer(
  'bullet',
  UNORDERED_LIST_REGEX,
);
export const ORDERED_LIST: ElementTransformer = createListTransformer(
  'number',
  ORDERED_LIST_REGEX,
);
export const CHECK_LIST: ElementTransformer = createListTransformer(
  'check',
  CHECK_LIST_REGEX,
);
