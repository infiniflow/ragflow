// The backend registers the Perplexity tool as `perplexity` with the canvas
// alias `perplexitysearch`; the model-facing name is `perplexity_search`.
export function isPerplexityOperator(value: unknown): boolean {
  return (
    typeof value === 'string' &&
    ['perplexity', 'perplexitysearch'].includes(
      value.trim().replace(/_/g, '').toLowerCase(),
    )
  );
}

// Use agentToolObject precedence; blank names fall through.
export function embeddedToolName(tool: unknown): string | undefined {
  if (!tool || typeof tool !== 'object') {
    return undefined;
  }
  const record = tool as Record<string, unknown>;
  for (const field of ['component_name', 'tool_name', 'name']) {
    const candidate = record[field];
    if (typeof candidate === 'string' && candidate.trim()) {
      return candidate.trim();
    }
  }
  return undefined;
}
