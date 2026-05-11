/**
 * Utility functions for extracting parser and raptor config extensions.
 * These functions extract known fields from parser/raptor config objects
 * and merge unknown fields into the `ext` field for flexible configuration.
 */

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
    auto_disable_for_structured_data,
    ext,
    ...raptorExt
  } = raptorConfig;
  return {
    use_raptor,
    prompt,
    max_token,
    threshold,
    max_cluster,
    random_seed,
    scope,
    auto_disable_for_structured_data,
    ext: { ...ext, ...raptorExt },
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
  } = parserConfig;
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
    parent_child: enable_children
      ? {
          children_delimiter,
          use_parent_child: use_parent_child ?? enable_children,
        }
      : undefined,
    ext: { ...ext, ...parserExt },
  };
};
