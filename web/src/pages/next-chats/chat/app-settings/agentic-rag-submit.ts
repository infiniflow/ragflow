import type { AgenticRagOverride } from '@/interfaces/database/chat';

/**
 * Maps the form's agentic_rag value to the PUT payload. The backend treats
 * `agentic_rag: null` as "reset to the shipped template", so blank content and
 * content identical to the server default both collapse to null — this keeps
 * chats on the shipped default following server-side template updates instead
 * of freezing a redundant copy. A stored tools override is preserved on every
 * path: the full-replace PUT must not silently drop it.
 */
export function resolveAgenticRagSubmitValue(
  value: AgenticRagOverride | null | undefined,
  defaultContent: string | undefined,
): AgenticRagOverride | null {
  const content = value?.content;
  const tools = value?.tools?.length ? value.tools : undefined;

  if (!content?.trim()) {
    return tools ? { tools } : null;
  }
  if (defaultContent !== undefined && content === defaultContent) {
    return tools ? { tools } : null;
  }
  return tools ? { content, tools } : { content };
}
