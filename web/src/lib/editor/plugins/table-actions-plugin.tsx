/**
 * TableActionsPlugin — floating toolbar for table editing.
 * Appears above the table when cursor is inside it.
 * Provides: row/column count adjustment, text alignment.
 */
import { useLexicalComposerContext } from '@lexical/react/LexicalComposerContext';
import {
  $createTableCellNode,
  $createTableRowNode,
  $isTableCellNode,
  $isTableNode,
  $isTableRowNode,
  $removeTableRowAtIndex,
  TableCellHeaderStates,
  type TableNode,
} from '@lexical/table';
import {
  $findMatchingParent,
  $getNodeByKey,
  $getSelection,
  $isElementNode,
  $isParagraphNode,
  $isRangeSelection,
  COMMAND_PRIORITY_LOW,
  SELECTION_CHANGE_COMMAND,
} from 'lexical';
import { useCallback, useEffect, useState } from 'react';

interface TableInfo {
  key: string;
  rows: number;
  cols: number;
}

/** Read-only: check if selection is inside a table and return its info. */
function $readTableInfo(): TableInfo | null {
  const selection = $getSelection();
  if (!$isRangeSelection(selection)) return null;
  const cellNode = $findMatchingParent(
    selection.anchor.getNode(),
    $isTableCellNode,
  );
  if (!cellNode) return null;
  const table = $findMatchingParent(cellNode, $isTableNode) as TableNode | null;
  if (!table) return null;
  const rows = table.getChildren().length;
  const firstRow = table.getFirstChild();
  const cols = $isTableRowNode(firstRow) ? firstRow.getChildren().length : 0;
  return { key: table.getKey(), rows, cols };
}

export default function TableActionsPlugin() {
  const [editor] = useLexicalComposerContext();
  const [show, setShow] = useState(false);
  const [rows, setRows] = useState(0);
  const [cols, setCols] = useState(0);
  const [tableKey, setTableKey] = useState<string | null>(null);
  const [pos, setPos] = useState({ x: 0, y: 0 });

  const checkTable = useCallback(() => {
    const info: TableInfo | null = editor.getEditorState().read(() => {
      return $readTableInfo();
    });

    if (!info) {
      setShow(false);
      setTableKey(null);
      return;
    }

    const { key, rows: r, cols: c } = info;
    setTableKey(key);
    setRows(r);
    setCols(c);

    // Try to position toolbar above the table's DOM element
    try {
      editor.read(() => {
        const domEl = editor.getElementByKey(key);
        if (domEl) {
          const tableRect = domEl.getBoundingClientRect();
          setPos({ x: tableRect.left, y: tableRect.top - 36 });
          setShow(true);
        }
      });
    } catch {
      // DOM not ready yet
    }
  }, [editor]);

  useEffect(() => {
    const removeUpdateListener = editor.registerUpdateListener(() => {
      checkTable();
    });

    const removeSelectionListener = editor.registerCommand(
      SELECTION_CHANGE_COMMAND,
      () => {
        checkTable();
        return false;
      },
      COMMAND_PRIORITY_LOW,
    );

    return () => {
      removeUpdateListener();
      removeSelectionListener();
    };
  }, [editor, checkTable]);

  const handleAddRow = useCallback(() => {
    if (!tableKey) return;
    editor.update(() => {
      const node = $getNodeByKey(tableKey);
      if (!$isTableNode(node)) return;
      const firstRow = node.getFirstChild();
      if (!$isTableRowNode(firstRow)) return;
      const colCount = firstRow.getChildren().length;
      const newRow = $createTableRowNode();
      for (let i = 0; i < colCount; i++) {
        newRow.append($createTableCellNode(TableCellHeaderStates.NO_STATUS));
      }
      node.append(newRow);
    });
  }, [editor, tableKey]);

  const handleRemoveRow = useCallback(() => {
    if (!tableKey || rows <= 1) return;
    editor.update(() => {
      const node = $getNodeByKey(tableKey);
      if (!$isTableNode(node)) return;
      $removeTableRowAtIndex(node, rows - 1);
    });
  }, [editor, tableKey, rows]);

  const handleAddCol = useCallback(() => {
    if (!tableKey) return;
    editor.update(() => {
      const node = $getNodeByKey(tableKey);
      if (!$isTableNode(node)) return;
      for (const row of node.getChildren()) {
        if ($isTableRowNode(row)) {
          row.append($createTableCellNode(TableCellHeaderStates.NO_STATUS));
        }
      }
    });
  }, [editor, tableKey]);

  const handleRemoveCol = useCallback(() => {
    if (!tableKey || cols <= 1) return;
    editor.update(() => {
      const node = $getNodeByKey(tableKey);
      if (!$isTableNode(node)) return;
      for (const row of node.getChildren()) {
        if ($isTableRowNode(row)) {
          const lastCell = row.getLastChild();
          lastCell?.remove();
        }
      }
    });
  }, [editor, tableKey, cols]);

  const setAlignment = useCallback(
    (align: 'left' | 'center' | 'right') => {
      if (!tableKey) return;
      editor.update(() => {
        const node = $getNodeByKey(tableKey);
        if (!$isTableNode(node)) return;
        for (const row of node.getChildren()) {
          if (!$isTableRowNode(row)) continue;
          for (const cell of row.getChildren()) {
            if (!$isTableCellNode(cell)) continue;
            for (const child of cell.getChildren()) {
              if ($isParagraphNode(child) || $isElementNode(child)) {
                child.setFormat(align);
              }
            }
          }
        }
      });
    },
    [editor, tableKey],
  );

  if (!show) return null;

  const btnStyle: React.CSSProperties = {
    padding: '4px 7px',
    border: 'none',
    borderRadius: 4,
    cursor: 'pointer',
    fontSize: 11,
    fontWeight: 500,
    background: 'transparent',
    color: 'var(--nim-text)',
    fontFamily: 'inherit',
    lineHeight: 1,
  };

  const countStyle: React.CSSProperties = {
    padding: '2px 6px',
    fontSize: 12,
    fontWeight: 600,
    color: 'var(--nim-text)',
    minWidth: 22,
    textAlign: 'center',
  };

  return (
    <div
      style={{
        position: 'fixed',
        left: pos.x,
        top: pos.y,
        zIndex: 100,
        display: 'flex',
        alignItems: 'center',
        gap: 2,
        padding: '3px 6px',
        background: 'var(--nim-bg-secondary)',
        border: '1px solid var(--nim-border)',
        borderRadius: 6,
        boxShadow: '0 2px 8px rgba(0,0,0,0.25)',
        pointerEvents: 'auto',
      }}
    >
      <svg
        width="13"
        height="13"
        viewBox="0 0 16 16"
        style={{
          fill: 'none',
          stroke: 'var(--nim-text-muted)',
          strokeWidth: 1.5,
          verticalAlign: 'middle',
          marginRight: 4,
          flexShrink: 0,
        }}
      >
        <rect x="1.5" y="1.5" width="13" height="13" rx="1.5" />
        <line x1="1.5" y1="6" x2="14.5" y2="6" />
        <line x1="8" y1="1.5" x2="8" y2="14.5" />
      </svg>

      {/* Row controls */}
      <button
        onClick={handleRemoveRow}
        disabled={rows <= 1}
        style={{ ...btnStyle, opacity: rows <= 1 ? 0.35 : 1 }}
        title="Delete row"
      >
        −R
      </button>
      <span style={countStyle}>{rows}</span>
      <button onClick={handleAddRow} style={btnStyle} title="Add row">
        +R
      </button>

      <div
        style={{
          width: 1,
          height: 14,
          background: 'var(--nim-border)',
          margin: '0 3px',
        }}
      />

      {/* Column controls */}
      <button
        onClick={handleRemoveCol}
        disabled={cols <= 1}
        style={{ ...btnStyle, opacity: cols <= 1 ? 0.35 : 1 }}
        title="Delete column"
      >
        −C
      </button>
      <span style={countStyle}>{cols}</span>
      <button onClick={handleAddCol} style={btnStyle} title="Add column">
        +C
      </button>

      <div
        style={{
          width: 1,
          height: 14,
          background: 'var(--nim-border)',
          margin: '0 3px',
        }}
      />

      {/* Alignment */}
      <button
        onClick={() => setAlignment('left')}
        style={btnStyle}
        title="Align left"
      >
        ⇤
      </button>
      <button
        onClick={() => setAlignment('center')}
        style={btnStyle}
        title="Align center"
      >
        ⇔
      </button>
      <button
        onClick={() => setAlignment('right')}
        style={btnStyle}
        title="Align right"
      >
        ⇥
      </button>
    </div>
  );
}
