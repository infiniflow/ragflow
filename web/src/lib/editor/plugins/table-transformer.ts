/**
 * Table transformer for markdown import/export
 */

/* eslint-disable @typescript-eslint/no-unused-vars, no-console, @typescript-eslint/no-use-before-define */

import { ElementTransformer } from '@lexical/markdown';
import {
  $createTableCellNode,
  $createTableNode,
  $createTableRowNode,
  $isTableCellNode,
  $isTableNode,
  $isTableRowNode,
  TableCellHeaderStates,
  TableCellNode,
  TableNode,
  TableRowNode,
} from '@lexical/table';
import { $isParagraphNode, $isTextNode, LexicalNode } from 'lexical';
import {
  $convertFromEnhancedMarkdownString,
  $convertNodeToEnhancedMarkdownString,
} from '../markdown';
import { isTableRowOversized } from './table-row-size-guard';

// Import transformers from editor
import { getEditorTransformers } from '../markdown';

// Get the complete set of transformers including core and plugin transformers
const getTransformers = () => {
  return getEditorTransformers();
};

// Very primitive table setup
const TABLE_ROW_REG_EXP = /^(?:\|)(.+)(?:\|)\s?$/;
const TABLE_ROW_DIVIDER_REG_EXP = /^(\| ?:?-*:? ?)+\|\s?$/;

export const TABLE_TRANSFORMER: ElementTransformer = {
  dependencies: [TableNode, TableRowNode, TableCellNode],
  export: (node: LexicalNode, _exportChildren) => {
    if (!$isTableNode(node)) {
      return null;
    }

    const output: string[] = [];
    const rows = node.getChildren();

    for (const row of rows) {
      const rowOutput = [];
      if (!$isTableRowNode(row)) {
        continue;
      }

      let isHeaderRow = false;
      for (const cell of row.getChildren()) {
        // It's TableCellNode so it's just to make flow happy
        if ($isTableCellNode(cell)) {
          // Use $convertNodeToEnhancedMarkdownString for single nodes
          rowOutput.push(
            $convertNodeToEnhancedMarkdownString(getTransformers(), cell)
              .replace(/\n/g, '\\n')
              .trim(),
          );
          if (cell.__headerState === TableCellHeaderStates.ROW) {
            isHeaderRow = true;
          }
        }
      }

      output.push(`| ${rowOutput.join(' | ')} |`);
      if (isHeaderRow) {
        output.push(`| ${rowOutput.map((_) => '---').join(' | ')} |`);
      }
    }

    return output.join('\n');
  },
  regExp: TABLE_ROW_REG_EXP,
  replace: (parentNode, _1, match) => {
    console.log('[TableTransformer] replace called, match:', match[0]);
    // Header row
    if (TABLE_ROW_DIVIDER_REG_EXP.test(match[0])) {
      const table = parentNode.getPreviousSibling();
      if (!table || !$isTableNode(table)) {
        return;
      }

      const rows = table.getChildren();
      const lastRow = rows[rows.length - 1];
      if (!lastRow || !$isTableRowNode(lastRow)) {
        return;
      }

      // Add header state to row cells
      lastRow.getChildren().forEach((cell) => {
        if (!$isTableCellNode(cell)) {
          return;
        }
        cell.setHeaderStyles(
          TableCellHeaderStates.ROW,
          TableCellHeaderStates.ROW,
        );
      });

      // Remove line
      parentNode.remove();
      return;
    }

    const matchCells = mapToTableCells(match[0]);

    if (matchCells == null) {
      return;
    }

    const rows = [matchCells];
    let sibling = parentNode.getPreviousSibling();
    let maxCells = matchCells.length;

    while (sibling) {
      if (!$isParagraphNode(sibling)) {
        break;
      }

      if (sibling.getChildrenSize() !== 1) {
        break;
      }

      const firstChild = sibling.getFirstChild();

      if (!$isTextNode(firstChild)) {
        break;
      }

      const cells = mapToTableCells(firstChild.getTextContent());

      if (cells == null) {
        break;
      }

      maxCells = Math.max(maxCells, cells.length);
      rows.unshift(cells);
      const previousSibling = sibling.getPreviousSibling();
      sibling.remove();
      sibling = previousSibling;
    }

    const table = $createTableNode();

    for (const cells of rows) {
      const tableRow = $createTableRowNode();
      table.append(tableRow);

      for (let i = 0; i < maxCells; i++) {
        tableRow.append(i < cells.length ? cells[i] : $createTableCell(''));
      }
    }

    const previousSibling = parentNode.getPreviousSibling();
    if (
      $isTableNode(previousSibling) &&
      getTableColumnsSize(previousSibling) === maxCells
    ) {
      previousSibling.append(...table.getChildren());
      parentNode.remove();
    } else {
      try {
        parentNode.replace(table);
      } catch (e) {
        console.error('Error replacing node with table:', match, e);
        throw e;
      }
    }
  },
  type: 'element',
};

function getTableColumnsSize(table: TableNode) {
  const row = table.getFirstChild();
  return $isTableRowNode(row) ? row.getChildrenSize() : 0;
}

const $createTableCell = (textContent: string): TableCellNode => {
  console.log(
    '[TableTransformer] $createTableCell with:',
    JSON.stringify(textContent),
  );
  textContent = textContent.replace(/\\n/g, '\n');
  const cell = $createTableCellNode(TableCellHeaderStates.NO_STATUS);
  $convertFromEnhancedMarkdownString(
    textContent,
    getTransformers(),
    cell,
    true,
  );
  console.log(
    '[TableTransformer] cell created, children:',
    cell.getChildrenSize(),
  );
  return cell;
};

const mapToTableCells = (textContent: string): Array<TableCellNode> | null => {
  if (isTableRowOversized(textContent)) {
    console.log('[TableTransformer] row oversized:', textContent.length);
    return null;
  }
  const match = textContent.match(TABLE_ROW_REG_EXP);
  if (!match || !match[1]) {
    console.log('[TableTransformer] no regex match for:', textContent);
    return null;
  }
  console.log('[TableTransformer] matched row:', match[1]);
  const cells = match[1].split('|').map((text) => {
    // Remove exactly one space from start and end if present
    const trimmed = text.replace(/^ | $/g, '');
    console.log(
      '[TableTransformer] cell text:',
      JSON.stringify(text),
      '->',
      JSON.stringify(trimmed),
    );
    return $createTableCell(trimmed);
  });
  console.log('[TableTransformer] created', cells.length, 'cells');
  return cells;
};
