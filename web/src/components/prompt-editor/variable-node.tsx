import { DecoratorNode, LexicalNode, NodeKey } from 'lexical';
import { ReactNode } from 'react';
import { Badge } from '../ui/badge';

export class VariableNode extends DecoratorNode<ReactNode> {
  __value: string;
  __label: string;

  static getType(): string {
    return 'variable';
  }

  static clone(node: VariableNode): VariableNode {
    return new VariableNode(node.__value, node.__label, node.__key);
  }

  constructor(value: string, label: string, key?: NodeKey) {
    super(key);
    this.__value = value;
    this.__label = label;
  }

  createDOM(): HTMLElement {
    const dom = document.createElement('span');
    dom.className = 'mr-1';

    return dom;
  }

  updateDOM(): false {
    return false;
  }

  decorate(): ReactNode {
    return <Badge>{this.__label}</Badge>;
  }

  getTextContent(): string {
    return `{${this.__value}}`;
  }
}

export function $createVariableNode(
  value: string,
  label: string,
): VariableNode {
  return new VariableNode(value, label);
}

export function $isVariableNode(
  node: LexicalNode | null | undefined,
): node is VariableNode {
  return node instanceof VariableNode;
}
