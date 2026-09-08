/**
 * Lexical nodes for the editor.
 * Each node type must be registered in the LexicalComposer config.
 */

import { CodeHighlightNode, CodeNode } from '@lexical/code';
import { HashtagNode } from '@lexical/hashtag';
import { AutoLinkNode, LinkNode } from '@lexical/link';
import { ListItemNode, ListNode } from '@lexical/list';
import { HeadingNode, QuoteNode } from '@lexical/rich-text';
import { TableCellNode, TableNode, TableRowNode } from '@lexical/table';
import type { Klass, LexicalNode } from 'lexical';
import { HorizontalRuleNode } from './markdown/horizontal-rule-node';
import { MermaidNode } from './mermaid-node';

const EditorNodes: Array<Klass<LexicalNode>> = [
  HeadingNode,
  QuoteNode,
  CodeNode,
  CodeHighlightNode,
  ListItemNode,
  ListNode,
  LinkNode,
  AutoLinkNode,
  HashtagNode,
  HorizontalRuleNode,
  MermaidNode,
  TableNode,
  TableRowNode,
  TableCellNode,
];

export default EditorNodes;
