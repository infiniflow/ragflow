import { $getSelection, $isRangeSelection, TextNode } from 'lexical';
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

  // Don't merge while the user may still be typing the path.
  //
  // Case 1: The path suffix reaches the end of the text (no remaining text).
  //         The user may type more identifier characters to extend the path.
  //
  // Case 2: The cursor sits right after the matched path suffix even though
  //         there is trailing text (e.g. a space left over from a previous
  //         merge). This means the user is actively typing the path before
  //         that pre-existing text, and the path may still grow. Merging now
  //         would lock the path to its current length.
  if (leadingPath.remainingText === '') {
    return false;
  }

  const selection = $getSelection();
  if (
    $isRangeSelection(selection) &&
    selection.isCollapsed() &&
    selection.anchor.key === textNode.getKey() &&
    selection.anchor.offset === leadingPath.pathSuffix.length
  ) {
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
