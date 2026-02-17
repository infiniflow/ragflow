import { DecoratorNode, LexicalNode, NodeKey } from 'lexical';
import { ReactNode } from 'react';

export class VariableNode extends DecoratorNode<ReactNode> {
  __value: string;
  __label: string;
  key?: NodeKey;
  __parentLabel?: string | ReactNode;
  __icon?: ReactNode;

  static getType(): string {
    return 'variable';
  }

  static clone(node: VariableNode): VariableNode {
    return new VariableNode(
      node.__value,
      node.__label,
      node.__key,
      node.__parentLabel,
      node.__icon,
    );
  }

  constructor(
    value: string,
    label: string,
    key?: NodeKey,
    parent?: string | ReactNode,
    icon?: ReactNode,
  ) {
    super(key);
    this.__value = value;
    this.__label = label;
    this.__parentLabel = parent;
    this.__icon = icon;
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
    let content: ReactNode = (
      <div className="text-accent-primary">{this.__label}</div>
    );
    if (this.__parentLabel) {
      content = (
        <div className="flex items-center gap-1 text-text-primary ">
          <div>{this.__icon}</div>
          <div>{this.__parentLabel}</div>
          <div className="text-text-disabled mr-1">/</div>
          {content}
        </div>
      );
    }
    return (
      <div className="bg-accent-primary-5 text-sm inline-flex items-center rounded-md px-2 py-1">
        {content}
      </div>
    );
  }

  getTextContent(): string {
    return `{${this.__value}}`;
  }
}

export function $createVariableNode(
  value: string,
  label: string,
  parentLabel: string | ReactNode,
  icon?: ReactNode,
): VariableNode {
  return new VariableNode(value, label, undefined, parentLabel, icon);
}

export function $isVariableNode(
  node: LexicalNode | null | undefined,
): node is VariableNode {
  return node instanceof VariableNode;
}
