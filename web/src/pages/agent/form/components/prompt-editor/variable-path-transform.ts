import { TextNode } from 'lexical';
import {
  appendPromptVariablePath,
  extractLeadingPromptVariablePath,
} from './utils';
import { $createVariableNode, $isVariableNode } from './variable-node';

export function mergeLeadingVariablePathTextNode(textNode: TextNode): boolean {
  const previousSibling = textNode.getPreviousSibling();

  if (!$isVariableNode(previousSibling)) {
    return false;
  }

  const leadingPath = extractLeadingPromptVariablePath(
    textNode.getTextContent(),
  );
  if (!leadingPath) {
    return false;
  }

  const nextVariable = appendPromptVariablePath(
    {
      value: previousSibling.__value,
      label: previousSibling.__label,
      parentLabel: previousSibling.__parentLabel,
      icon: previousSibling.__icon,
    },
    leadingPath.pathSuffix,
  );

  previousSibling.replace(
    $createVariableNode(
      nextVariable.value,
      nextVariable.label,
      nextVariable.parentLabel,
      nextVariable.icon,
    ),
  );
  textNode.setTextContent(leadingPath.remainingText);

  return true;
}
