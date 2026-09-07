import { DiagramCodeBlock } from '@/components/diagram-code-block';
import { AlertTriangle, FileText, MousePointer2 } from 'lucide-react';
import { isValidElement, useEffect, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type {
  BusinessDocumentRevision,
  BusinessDocumentSelection,
} from '../types';

interface DocumentPaneProps {
  revision: BusinessDocumentRevision | null;
  onSelectionChange: (selection: BusinessDocumentSelection | null) => void;
}

const CONTEXT_WINDOW = 64;
const SECTION_SELECTOR = '[data-section-id][data-section-text]';

function isHighSurrogate(value: number) {
  return value >= 0xd800 && value <= 0xdbff;
}

function isLowSurrogate(value: number) {
  return value >= 0xdc00 && value <= 0xdfff;
}

function isUtf16Boundary(text: string, offset: number) {
  return !(
    offset > 0 &&
    offset < text.length &&
    isHighSurrogate(text.charCodeAt(offset - 1)) &&
    isLowSurrogate(text.charCodeAt(offset))
  );
}

function contextForSelection(
  text: string,
  startOffset: number,
  endOffset: number,
) {
  let prefixStart = Math.max(0, startOffset - CONTEXT_WINDOW);
  if (!isUtf16Boundary(text, prefixStart)) prefixStart += 1;

  let suffixEnd = Math.min(text.length, endOffset + CONTEXT_WINDOW);
  if (!isUtf16Boundary(text, suffixEnd)) suffixEnd -= 1;

  return {
    prefix: text.slice(prefixStart, startOffset),
    suffix: text.slice(endOffset, suffixEnd),
  };
}

function sectionForNode(node: Node | null) {
  const element =
    node instanceof Element ? node : (node?.parentElement ?? null);
  return element?.closest<HTMLElement>(SECTION_SELECTOR) ?? null;
}

function findUniqueOffset(sectionText: string, selectedText: string) {
  const startOffset = sectionText.indexOf(selectedText);
  if (startOffset < 0) return { error: 'NOT_EXACT' as const };
  if (sectionText.indexOf(selectedText, startOffset + 1) >= 0) {
    return { error: 'AMBIGUOUS' as const };
  }
  return { startOffset };
}

export function DocumentPane({
  revision,
  onSelectionChange,
}: DocumentPaneProps) {
  const paneRef = useRef<HTMLDivElement>(null);
  const [selectionError, setSelectionError] = useState<string | null>(null);

  useEffect(() => setSelectionError(null), [revision?.revision_id]);

  const rejectSelection = (message: string) => {
    onSelectionChange(null);
    setSelectionError(message);
  };

  const captureSelection = () => {
    const browserSelection = window.getSelection();
    const range = browserSelection?.rangeCount
      ? browserSelection.getRangeAt(0)
      : null;
    const selectedText = browserSelection?.toString() ?? '';

    if (!range || !selectedText.trim()) {
      onSelectionChange(null);
      setSelectionError(null);
      return;
    }

    const pane = paneRef.current;
    const startSection = sectionForNode(range.startContainer);
    const endSection = sectionForNode(range.endContainer);
    if (
      !pane?.contains(range.commonAncestorContainer) ||
      !startSection ||
      !endSection ||
      startSection !== endSection
    ) {
      rejectSelection('Выделите фрагмент внутри одного раздела.');
      return;
    }

    const sectionId = startSection.dataset.sectionId;
    const sectionText = startSection.dataset.sectionText;
    if (!sectionId || sectionText === undefined) {
      rejectSelection('Не удалось определить раздел для комментария.');
      return;
    }

    const match = findUniqueOffset(sectionText, selectedText);
    if ('error' in match) {
      rejectSelection(
        match.error === 'AMBIGUOUS'
          ? 'Фрагмент повторяется в разделе. Выделите более длинный текст.'
          : 'Выделение не совпадает с каноническим текстом раздела.',
      );
      return;
    }

    const endOffset = match.startOffset + selectedText.length;
    if (
      !isUtf16Boundary(sectionText, match.startOffset) ||
      !isUtf16Boundary(sectionText, endOffset)
    ) {
      rejectSelection(
        'Выделение разрывает составной символ. Выделите его целиком.',
      );
      return;
    }
    const context = contextForSelection(
      sectionText,
      match.startOffset,
      endOffset,
    );
    onSelectionChange({
      revision_id: revision!.revision_id,
      section_id: sectionId,
      selected_text: selectedText,
      prefix: context.prefix,
      suffix: context.suffix,
      start_offset: match.startOffset,
      end_offset: endOffset,
    });
    setSelectionError(null);
  };

  return (
    <section
      className="flex min-h-0 flex-col bg-bg-base"
      data-testid="business-document-pane"
      aria-label="Документ"
    >
      <div className="flex h-11 shrink-0 items-center justify-between border-b border-border-button px-5">
        <div className="flex items-center gap-2 text-sm font-medium text-text-primary">
          <FileText className="size-4 text-text-secondary" />
          Документ
        </div>
        {revision && (
          <span className="font-mono text-xs text-text-secondary">
            Ревизия {revision.revision_number}
          </span>
        )}
      </div>

      {!revision ? (
        <div
          className="flex min-h-0 flex-1 items-center justify-center px-8"
          data-testid="business-document-empty"
        >
          <div className="max-w-sm text-center">
            <FileText className="mx-auto size-8 stroke-[1.25] text-text-disabled" />
            <h2 className="mt-4 text-base font-medium text-text-primary">
              Черновик ещё не создан
            </h2>
            <p className="mt-2 text-sm leading-6 text-text-secondary">
              Ответьте на вопросы справа. Когда вводных будет достаточно, агент
              подготовит первую ревизию.
            </p>
          </div>
        </div>
      ) : (
        <div
          ref={paneRef}
          onMouseUp={captureSelection}
          className="min-h-0 flex-1 overflow-y-auto px-6 py-8 scrollbar-auto md:px-10 lg:px-14"
          data-testid="business-document-markdown"
        >
          <article className="mx-auto max-w-[860px] text-[15px] leading-7 text-text-primary">
            {revision.document_ast.sections.map((section) => {
              const sectionText = revision.section_texts[section.id] ?? '';
              const displayedSectionText = sectionText.trim()
                ? sectionText
                : 'Требования отсутствуют';
              const headingLevel = Math.min(
                6,
                2 + section.id.split('.').length - 1,
              );
              const markdown = `${'#'.repeat(headingLevel)} ${section.id}. ${section.title}\n\n${displayedSectionText}`;

              return (
                <section
                  key={section.id}
                  data-section-id={section.id}
                  data-section-text={sectionText}
                  data-testid="business-document-section"
                >
                  <ReactMarkdown
                    remarkPlugins={[remarkGfm]}
                    components={{
                      h2: ({ children }) => (
                        <h2 className="mb-3 mt-9 border-b border-border-button pb-2 text-xl font-semibold tracking-tight first:mt-0">
                          {children}
                        </h2>
                      ),
                      h3: ({ children }) => (
                        <h3 className="mb-2 mt-6 text-base font-semibold">
                          {children}
                        </h3>
                      ),
                      p: ({ children }) => <p className="mb-4">{children}</p>,
                      ul: ({ children }) => (
                        <ul className="mb-4 list-disc space-y-1 ps-6">
                          {children}
                        </ul>
                      ),
                      ol: ({ children }) => (
                        <ol className="mb-4 list-decimal space-y-1 ps-6">
                          {children}
                        </ol>
                      ),
                      blockquote: ({ children }) => (
                        <blockquote className="my-5 border-s-2 border-accent-primary ps-4 text-text-secondary">
                          {children}
                        </blockquote>
                      ),
                      table: ({ children }) => (
                        <div className="mb-5 overflow-x-auto">
                          <table className="w-full border-collapse text-sm">
                            {children}
                          </table>
                        </div>
                      ),
                      th: ({ children }) => (
                        <th className="border-b border-border-default bg-bg-card px-3 py-2 text-start font-medium">
                          {children}
                        </th>
                      ),
                      td: ({ children }) => (
                        <td className="border-b border-border-button px-3 py-2 align-top">
                          {children}
                        </td>
                      ),
                      pre: ({ children }) => {
                        const code = isValidElement<{
                          className?: string;
                          children?: unknown;
                        }>(children)
                          ? children
                          : null;
                        if (
                          code?.props.className
                            ?.split(/\s+/)
                            .includes('language-plantuml')
                        ) {
                          return (
                            <DiagramCodeBlock
                              language="plantuml"
                              source={String(code.props.children ?? '').replace(
                                /\n$/,
                                '',
                              )}
                            />
                          );
                        }
                        return (
                          <pre className="mb-4 overflow-x-auto rounded bg-bg-card p-3 text-sm">
                            {children}
                          </pre>
                        );
                      },
                      code: ({ children }) => (
                        <code className="rounded bg-bg-card px-1.5 py-0.5 font-mono text-[0.9em]">
                          {children}
                        </code>
                      ),
                      a: ({ children, href }) => (
                        <a
                          href={href}
                          target="_blank"
                          rel="noreferrer noopener"
                          className="text-accent-primary underline-offset-4 hover:underline"
                        >
                          {children}
                        </a>
                      ),
                    }}
                  >
                    {markdown}
                  </ReactMarkdown>
                </section>
              );
            })}
          </article>
        </div>
      )}

      {revision && (
        <div
          className={`flex shrink-0 items-center gap-2 border-t px-5 py-2 text-xs ${
            selectionError
              ? 'border-state-warning/40 bg-state-warning/5 text-state-warning'
              : 'border-border-button text-text-secondary'
          }`}
          role={selectionError ? 'alert' : undefined}
          data-testid={
            selectionError
              ? 'business-document-selection-error'
              : 'business-document-selection-help'
          }
        >
          {selectionError ? (
            <AlertTriangle className="size-3.5 shrink-0" />
          ) : (
            <MousePointer2 className="size-3.5" />
          )}
          {selectionError ??
            'Выделите фрагмент, чтобы привязать к нему комментарий'}
        </div>
      )}
    </section>
  );
}
