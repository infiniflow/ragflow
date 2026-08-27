/**
 * Custom HorizontalRuleNode for Lexical.
 * Uses ElementNode (no decorative rendering needed).
 */

import {
  $applyNodeReplacement,
  ElementNode,
  type EditorConfig,
  type ElementFormatType,
  type LexicalNode,
  type SerializedLexicalNode,
  type Spread,
} from 'lexical';

export type SerializedHorizontalRuleNode = Spread<
  {
    children: [];
    direction: 'ltr' | 'rtl' | null;
    format: ElementFormatType;
    indent: number;
  },
  SerializedLexicalNode
>;

export class HorizontalRuleNode extends ElementNode {
  static getType(): string {
    return 'horizontalrule';
  }

  static clone(node: HorizontalRuleNode): HorizontalRuleNode {
    return new HorizontalRuleNode(node.__key);
  }

  /* eslint-disable @typescript-eslint/no-unused-vars */
  static importJSON(
    _serializedNode: SerializedHorizontalRuleNode,
  ): HorizontalRuleNode {
    return $createHorizontalRuleNode();
  }
  /* eslint-enable @typescript-eslint/no-unused-vars */

  exportJSON(): SerializedHorizontalRuleNode {
    return {
      children: [],
      direction: 'ltr',
      format: '',
      indent: 0,
      type: 'horizontalrule',
      version: 1,
    };
  }

  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  createDOM(_config: EditorConfig, _editor: any): HTMLElement {
    const element = document.createElement('hr');
    return element;
  }

  updateDOM(): boolean {
    return false;
  }

  getTextContent(): string {
    return '\n';
  }

  isInline(): boolean {
    return false;
  }

  canBeEmpty(): boolean {
    return true;
  }
}

export function $createHorizontalRuleNode(): HorizontalRuleNode {
  return $applyNodeReplacement(new HorizontalRuleNode());
}

export function $isHorizontalRuleNode(
  node: LexicalNode | null | undefined,
): node is HorizontalRuleNode {
  return node instanceof HorizontalRuleNode;
}
