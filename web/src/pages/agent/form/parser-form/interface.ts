export type CommonProps = {
  prefix: string;
  // Whether the owning dataset declared the table parser. Table column settings
  // only take effect on that parser, so dataset-scoped callers pass their chunk
  // method and every other dataset renders the fields without them; undefined
  // (canvas editor) keeps the fields visible.
  isTableParser?: boolean;
};
