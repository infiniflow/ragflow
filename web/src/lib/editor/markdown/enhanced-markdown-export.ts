/**
 * Enhanced markdown export supporting individual node export and
 * proper handling of all node types.
 * Ported from Nimbalyst — simplified: no frontmatter, no diff/reject mode.
 */

import type {
  ElementTransformer,
  MultilineElementTransformer,
  TextFormatTransformer,
  TextMatchTransformer,
} from '@lexical/markdown';
import {
  $getRoot,
  $isDecoratorNode,
  $isElementNode,
  $isLineBreakNode,
  $isRootOrShadowRoot,
  $isTextNode,
  type ElementNode,
  type LexicalNode,
  type TextFormatType,
  type TextNode,
} from 'lexical';
export type LexicalTransformer =
  | ElementTransformer
  | MultilineElementTransformer
  | TextFormatTransformer
  | TextMatchTransformer;

type UnclosedFormatTag = { format: TextFormatType; tag: string };

export interface EnhancedExportOptions {
  shouldPreserveNewLines?: boolean;
}

export function $convertToEnhancedMarkdownString(
  transformers: LexicalTransformer[],
  options: EnhancedExportOptions = {},
): string {
  const { shouldPreserveNewLines = true } = options;

  return $exportNodeToMarkdown(
    transformers,
    $getRoot(),
    shouldPreserveNewLines,
  );
}

export function $convertNodeToEnhancedMarkdownString(
  transformers: LexicalTransformer[],
  node?: ElementNode | null,
  shouldPreserveNewLines: boolean = true,
): string {
  return $exportNodeToMarkdown(transformers, node, shouldPreserveNewLines);
}

function $exportNodeToMarkdown(
  transformers: LexicalTransformer[],
  node?: ElementNode | null,
  shouldPreserveNewLines: boolean = true,
): string {
  const byType = transformersByType(transformers);
  const isNewlineDelimited = !byType.multilineElement.length;

  const textFormatTransformers = byType.textFormat
    .filter((t) => t.format.length === 1)
    .sort(
      (a, b) =>
        Number(a.format.includes('code')) - Number(b.format.includes('code')),
    );

  const textMatchTransformers = byType.textMatch;
  const elementTransformers = [...byType.element, ...byType.multilineElement];

  const output: string[] = [];
  const target = node || $getRoot();

  if (
    target &&
    (!$isRootOrShadowRoot(target) || target.getType() === 'table')
  ) {
    const result = exportTopLevelElements(
      target,
      elementTransformers,
      textFormatTransformers,
      textMatchTransformers,
      shouldPreserveNewLines,
    );
    if (result !== null) output.push(result);
  } else {
    const children = target.getChildren();
    for (let i = 0; i < children.length; i++) {
      const child = children[i];
      const result = exportTopLevelElements(
        child,
        elementTransformers,
        textFormatTransformers,
        textMatchTransformers,
        shouldPreserveNewLines,
      );
      if (result !== null) {
        output.push(
          isNewlineDelimited &&
            i > 0 &&
            !isEmptyParagraph(child) &&
            !isEmptyParagraph(children[i - 1])
            ? '\n' + result
            : result,
        );
      }
    }
  }

  return output.join(shouldPreserveNewLines ? '\n' : '\n\n');
}

function exportTopLevelElements(
  node: LexicalNode,
  elementTransformers: Array<ElementTransformer | MultilineElementTransformer>,
  textFormatTransformers: Array<TextFormatTransformer>,
  textMatchTransformers: Array<TextMatchTransformer>,
  shouldPreserveNewLines: boolean = false,
): string | null {
  for (const transformer of elementTransformers) {
    if (!transformer.export) continue;
    const result = transformer.export(node, (_node) =>
      exportChildren(
        _node,
        textFormatTransformers,
        textMatchTransformers,
        undefined,
        undefined,
        shouldPreserveNewLines,
        elementTransformers,
      ),
    );
    if (result != null) return result;
  }

  if ($isElementNode(node)) {
    return exportChildren(
      node,
      textFormatTransformers,
      textMatchTransformers,
      undefined,
      undefined,
      shouldPreserveNewLines,
      elementTransformers,
    );
  } else if ($isDecoratorNode(node)) {
    return node.getTextContent();
  }
  return null;
}

function exportChildren(
  node: ElementNode,
  textFormatTransformers: Array<TextFormatTransformer>,
  textMatchTransformers: Array<TextMatchTransformer>,
  textContent?: string,
  textTransformer?: TextFormatTransformer | null,
  shouldPreserveNewLines: boolean = false,
  elementTransformers?: Array<ElementTransformer | MultilineElementTransformer>,
  unclosedTags?: Array<UnclosedFormatTag>,
  unclosableTags?: Array<UnclosedFormatTag>,
): string {
  const output: string[] = [];
  const children = node.getChildren();
  const activeUnclosedTags = unclosedTags ?? [];
  const activeUnclosableTags = unclosableTags ?? [];

  for (const child of children) {
    if ($isLineBreakNode(child)) {
      if (shouldPreserveNewLines) output.push('\n');
    } else if ($isTextNode(child)) {
      const textContentForTransform = textContent || child.getTextContent();

      if (textTransformer) {
        output.push(textContentForTransform);
      } else {
        const hasFormatting = child.getFormat() !== 0;
        let handled = false;

        if (hasFormatting) {
          output.push(
            exportTextFormat(
              child,
              textContentForTransform,
              textFormatTransformers,
              activeUnclosedTags,
              activeUnclosableTags,
              shouldPreserveNewLines,
            ),
          );
          handled = true;
        } else {
          for (const transformer of textMatchTransformers) {
            if (!transformer.export) continue;
            const result = transformer.export(
              child,
              (_node: ElementNode, textContent?: string) =>
                exportChildren(
                  _node,
                  textFormatTransformers,
                  textMatchTransformers,
                  textContent,
                  textTransformer,
                  shouldPreserveNewLines,
                  elementTransformers,
                  activeUnclosedTags,
                  [...activeUnclosableTags, ...activeUnclosedTags],
                ),
              (node: TextNode, textContent: string) =>
                exportTextFormat(
                  node,
                  textContent,
                  textFormatTransformers,
                  activeUnclosedTags,
                  activeUnclosableTags,
                  shouldPreserveNewLines,
                ),
            );
            if (result != null) {
              output.push(result);
              handled = true;
              break;
            }
          }
        }

        if (!handled) output.push(textContentForTransform);
      }
    } else if ($isElementNode(child)) {
      let handled = false;
      for (const transformer of textMatchTransformers) {
        if (!transformer.export) continue;
        const result = transformer.export(
          child,
          (_node: ElementNode) =>
            exportChildren(
              _node,
              textFormatTransformers,
              textMatchTransformers,
              undefined,
              undefined,
              shouldPreserveNewLines,
              elementTransformers,
              activeUnclosedTags,
              [...activeUnclosableTags, ...activeUnclosedTags],
            ),
          (node: TextNode, textContent: string) =>
            exportTextFormat(
              node,
              textContent,
              textFormatTransformers,
              activeUnclosedTags,
              activeUnclosableTags,
              shouldPreserveNewLines,
            ),
        );
        if (result != null) {
          output.push(result);
          handled = true;
          break;
        }
      }
      if (!handled) {
        const result = exportTopLevelElements(
          child,
          elementTransformers || [],
          textFormatTransformers,
          textMatchTransformers,
          shouldPreserveNewLines,
        );
        if (result != null) output.push(result);
      }
    } else if ($isDecoratorNode(child)) {
      let handled = false;
      for (const transformer of textMatchTransformers) {
        if (!transformer.export) continue;
        const result = transformer.export(
          child,
          (_node: ElementNode) =>
            exportChildren(
              _node,
              textFormatTransformers,
              textMatchTransformers,
              undefined,
              undefined,
              shouldPreserveNewLines,
              elementTransformers,
              activeUnclosedTags,
              [...activeUnclosableTags, ...activeUnclosedTags],
            ),
          (node: TextNode, textContent: string) =>
            exportTextFormat(
              node,
              textContent,
              textFormatTransformers,
              activeUnclosedTags,
              activeUnclosableTags,
              shouldPreserveNewLines,
            ),
        );
        if (result != null) {
          output.push(result);
          handled = true;
          break;
        }
      }
      if (!handled && elementTransformers) {
        for (const transformer of elementTransformers) {
          const result = transformer.export?.(child, () => '');
          if (result != null) {
            output.push(result);
            handled = true;
            break;
          }
        }
      }
      if (!handled) output.push(child.getTextContent());
    }
  }
  return output.join('');
}

function exportTextFormat(
  node: TextNode,
  textContent: string,
  textTransformers: Array<TextFormatTransformer>,
  unclosedTags: Array<UnclosedFormatTag>,
  unclosableTags: Array<UnclosedFormatTag>,
  shouldPreserveNewLines: boolean = false,
): string {
  let output = textContent;

  if (!node.hasFormat('code')) {
    if (shouldPreserveNewLines) {
      // Use HTML NCR for * and _ instead of backslash escapes (round-trip safe)
      output = output
        .replace(/\*/g, '&#42;')
        .replace(/_/g, '&#95;')
        .replace(/([`~])/g, '\\$1');
    } else {
      output = output.replace(/([*_`~\\])/g, '\\$1');
    }
  }

  const match = output.match(/^(\s*)(.*?)(\s*)$/s) || ['', '', output, ''];
  const leadingSpace = match[1];
  const trimmedOutput = match[2];
  const trailingSpace = match[3];
  const isWhitespaceOnly = trimmedOutput === '';

  let openingTags = '';
  let closingTagsBefore = '';
  let closingTagsAfter = '';
  const previousTextNode = getTextSibling(node, true);
  const nextTextNode = getTextSibling(node, false);
  const appliedFormats = new Set<TextFormatType>();

  for (const transformer of textTransformers) {
    const format = transformer.format[0];
    const tag = transformer.tag;
    if (hasTextFormat(node, format) && !appliedFormats.has(format)) {
      appliedFormats.add(format);
      if (
        !hasTextFormat(previousTextNode, format) ||
        !unclosedTags.find((entry) => entry.tag === tag)
      ) {
        unclosedTags.push({ format, tag });
        openingTags += tag;
      }
    }
  }

  for (let i = 0; i < unclosedTags.length; i++) {
    const currentTag = unclosedTags[i];
    const nodeHasFormat = hasTextFormat(node, currentTag.format);
    const nextNodeHasFormat = hasTextFormat(nextTextNode, currentTag.format);

    if (nodeHasFormat && nextNodeHasFormat) continue;

    const remainingTags = [...unclosedTags];
    while (remainingTags.length > i) {
      const tagToClose = remainingTags.pop();
      if (
        tagToClose &&
        unclosableTags.find((entry) => entry.tag === tagToClose.tag)
      )
        continue;
      if (tagToClose) {
        if (!nodeHasFormat) closingTagsBefore += tagToClose.tag;
        else if (!nextNodeHasFormat) closingTagsAfter += tagToClose.tag;
      }
      unclosedTags.pop();
    }
    break;
  }

  if (isWhitespaceOnly && !node.hasFormat('code')) {
    return closingTagsBefore + output;
  }

  return (
    closingTagsBefore +
    leadingSpace +
    openingTags +
    trimmedOutput +
    closingTagsAfter +
    trailingSpace
  );
}

function isEmptyParagraph(node: LexicalNode): boolean {
  if (!$isElementNode(node)) return false;
  const children = node.getChildren();
  if (children.length === 0) return true;
  if (children.length === 1) {
    const child = children[0];
    if ($isTextNode(child) && child.getTextContent().trim() === '') return true;
  }
  return false;
}

function transformersByType(transformers: LexicalTransformer[]) {
  const byType: {
    element: Array<ElementTransformer>;
    multilineElement: Array<MultilineElementTransformer>;
    textFormat: Array<TextFormatTransformer>;
    textMatch: Array<TextMatchTransformer>;
  } = { element: [], multilineElement: [], textFormat: [], textMatch: [] };

  for (const transformer of transformers) {
    const type = transformer.type;
    if (type === 'element')
      byType.element.push(transformer as ElementTransformer);
    else if (type === 'multiline-element')
      byType.multilineElement.push(transformer as MultilineElementTransformer);
    else if (type === 'text-format')
      byType.textFormat.push(transformer as TextFormatTransformer);
    else if (type === 'text-match')
      byType.textMatch.push(transformer as TextMatchTransformer);
  }
  return byType;
}

function getTextSibling(node: TextNode, backward: boolean): TextNode | null {
  let sibling = backward ? node.getPreviousSibling() : node.getNextSibling();
  if (!sibling) {
    const parent = node.getParent();
    if (parent?.isInline())
      sibling = backward
        ? parent.getPreviousSibling()
        : parent.getNextSibling();
  }
  while (sibling) {
    if ($isElementNode(sibling)) {
      if (!sibling.isInline()) break;
      const descendant = backward
        ? sibling.getLastDescendant()
        : sibling.getFirstDescendant();
      if ($isTextNode(descendant)) return descendant;
      sibling = backward
        ? sibling.getPreviousSibling()
        : sibling.getNextSibling();
      continue;
    }
    if ($isTextNode(sibling)) return sibling;
    break;
  }
  return null;
}

function hasTextFormat(
  node: LexicalNode | null | undefined,
  format: TextFormatType,
): boolean {
  return $isTextNode(node) && node.hasFormat(format);
}
