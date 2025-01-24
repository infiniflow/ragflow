import { DecoratorNode, LexicalNode, NodeKey } from 'lexical';
import { ReactNode } from 'react';
import { Badge } from '../ui/badge';

export class VariableNode extends DecoratorNode<ReactNode> {
  __id: string;

  static getType(): string {
    return 'video';
  }

  static clone(node: VariableNode): VariableNode {
    return new VariableNode(node.__id, node.__key);
  }

  constructor(id: string, key?: NodeKey) {
    super(key);
    this.__id = id;
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
    return <Badge>{this.__id}</Badge>;
  }
}

export function $createVariableNode(id: string): VariableNode {
  return new VariableNode(id);
}

export function $isVariableNode(
  node: LexicalNode | null | undefined,
): node is VariableNode {
  return node instanceof VariableNode;
}
