import i18n from '@/locales/config';
import { BeginId } from '@/pages/flow/constant';
import { DecoratorNode, LexicalNode, NodeKey } from 'lexical';
import { ReactNode } from 'react';
const prefix = BeginId + '@';

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
    let content: ReactNode = (
      <span className="text-blue-600">{this.__label}</span>
    );
    if (this.__value.startsWith(prefix)) {
      content = (
        <div>
          <span>{i18n.t(`flow.begin`)}</span> / {content}
        </div>
      );
    }
    return (
      <div className="bg-gray-200 dark:bg-gray-400 text-primary inline-flex items-center rounded-md px-2 py-0">
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
): VariableNode {
  return new VariableNode(value, label);
}

export function $isVariableNode(
  node: LexicalNode | null | undefined,
): node is VariableNode {
  return node instanceof VariableNode;
}
