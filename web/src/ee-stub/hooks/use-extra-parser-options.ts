/**
 * Enterprise seam for backend-contributed parser options. The enterprise
 * build returns the available EE doc-analyzer engines; open source keeps the
 * static parser list, so this extension is always empty.
 */
const NoExtraParserOptions: { value: string; label: string }[] = [];

export const useExtraParserOptions = () => NoExtraParserOptions;
