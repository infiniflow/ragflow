/**
 * Utility functions for extracting parser and raptor config extensions.
 * These functions extract known fields from parser/raptor config objects
 * and merge unknown fields into the `ext` field for flexible configuration.
 */

type ParentChildConfig = {
  use_parent_child?: boolean;
  children_delimiter?: string;
};

/**
 * Normalize parser_config from the API for react-hook-form.
 * The backend stores parent-child chunking under `parent_child` and/or
 * flattened `enable_children` + `children_delimiter`; the UI only edits
 * the flattened fields (issue #15578).
 */
export const normalizeInboundParserConfig = (
  parserConfig: Record<string, any> | undefined,
) => {
  if (!parserConfig) {
    return parserConfig;
  }

  const parentChild = parserConfig.parent_child as
    | ParentChildConfig
    | undefined;
  const enable_children = Boolean(
    parserConfig.enable_children ?? parentChild?.use_parent_child ?? false,
  );
  const children_delimiter =
    (enable_children
      ? (parserConfig.children_delimiter ?? parentChild?.children_delimiter)
      : (parserConfig.children_delimiter ?? parentChild?.children_delimiter)) ??
    '\n';

  return {
    ...parserConfig,
    enable_children,
    children_delimiter,
  };
};

/**
 * Pipeline parser configs are keyed by operator id (e.g. "Parser:xxx"), so a
 * top-level key containing ":" marks the pipeline structure, which must be
 * sent as-is instead of being reshaped by extractParserConfigExt.
 */
export const isPipelineParserConfig = (
  parserConfig: Record<string, any> | undefined,
): boolean => {
  if (!parserConfig || typeof parserConfig !== 'object') {
    return false;
  }
  return Object.keys(parserConfig).some((key) => key.includes(':'));
};

/**
 * Extracts Raptor configuration with extra fields merged into ext.
 * @param raptorConfig - The raptor configuration object
 * @returns Processed raptor config with extra fields in ext
 */
export const extractRaptorConfigExt = (
  raptorConfig: Record<string, any> | undefined,
) => {
  if (!raptorConfig) return raptorConfig;
  const {
    use_raptor,
    prompt,
    max_token,
    threshold,
    max_cluster,
    random_seed,
    scope,
    clustering_method,
    tree_builder,
    auto_disable_for_structured_data,
    ext,
    ...raptorExt
  } = raptorConfig;
  const extClusteringMethod = ext?.clustering_method;
  const normalizedClusteringMethod =
    clustering_method ?? extClusteringMethod ?? 'gmm';
  const normalizedTreeBuilder = tree_builder ?? ext?.tree_builder ?? 'raptor';

  return {
    use_raptor,
    prompt,
    max_token,
    threshold,
    max_cluster,
    random_seed,
    scope,
    auto_disable_for_structured_data,
    ext: {
      ...ext,
      ...raptorExt,
      clustering_method: normalizedClusteringMethod,
      tree_builder: normalizedTreeBuilder,
    },
  };
};

/**
 * Extracts Parser configuration with extra fields merged into ext.
 * @param parserConfig - The parser configuration object
 * @returns Processed parser config with extra fields in ext
 */
export const extractParserConfigExt = (
  parserConfig: Record<string, any> | undefined,
) => {
  if (!parserConfig) return parserConfig;
  const normalized = normalizeInboundParserConfig(parserConfig);
  const {
    auto_keywords,
    auto_questions,
    chunk_token_num,
    delimiter,
    graphrag,
    html4excel,
    layout_recognize,
    raptor,
    tag_kb_ids,
    topn_tags,
    filename_embd_weight,
    task_page_size,
    pages,
    children_delimiter,
    use_parent_child,
    enable_children,
    ext,
    ...parserExt
  } = normalized;
  return {
    auto_keywords,
    auto_questions,
    chunk_token_num,
    delimiter,
    graphrag,
    html4excel,
    layout_recognize,
    raptor: extractRaptorConfigExt(raptor),
    tag_kb_ids,
    topn_tags,
    filename_embd_weight,
    task_page_size,
    pages,
    children_delimiter,
    enable_children,
    parent_child: enable_children
      ? {
          children_delimiter,
          use_parent_child: use_parent_child ?? enable_children,
        }
      : undefined,
    ext: { ...ext, ...parserExt },
  };
};
