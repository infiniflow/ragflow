import { extractParserConfigExt } from '../parser-config-utils';

describe('extractParserConfigExt', () => {
  it('serializes RAPTOR clustering fields through ext for API compatibility', () => {
    const result = extractParserConfigExt({
      raptor: {
        use_raptor: true,
        prompt: 'Summarize {cluster_content}',
        max_token: 256,
        threshold: 0.1,
        max_cluster: 317,
        random_seed: 0,
        scope: 'file',
        clustering_method: 'ahc',
        tree_builder: 'raptor',
      },
    });

    expect(result?.raptor).not.toHaveProperty('clustering_method');
    expect(result?.raptor).not.toHaveProperty('tree_builder');
    expect(result?.raptor?.ext).toMatchObject({
      clustering_method: 'ahc',
      tree_builder: 'raptor',
    });
  });

  it('preserves existing RAPTOR ext clustering values when the top-level field is absent', () => {
    const result = extractParserConfigExt({
      raptor: {
        max_cluster: 512,
        ext: {
          clustering_method: 'ahc',
          tree_builder: 'raptor',
          psi_bucket_size: 1024,
        },
      },
    });

    expect(result?.raptor?.ext).toMatchObject({
      clustering_method: 'ahc',
      tree_builder: 'raptor',
      psi_bucket_size: 1024,
    });
  });

  it('keeps legacy parent-child fields out of ext when children are disabled', () => {
    const result = extractParserConfigExt({
      enable_children: false,
      children_delimiter: '\\n',
      parent_child: {
        children_delimiter: '\\n',
        use_parent_child: true,
      },
    });

    expect(result?.parent_child).toEqual({
      children_delimiter: '\\n',
      use_parent_child: true,
    });
    expect(result?.ext).not.toHaveProperty('parent_child');
  });

  it('prefers normalized parent-child fields when children are enabled', () => {
    const result = extractParserConfigExt({
      enable_children: true,
      children_delimiter: '\\n',
      use_parent_child: false,
      parent_child: {
        children_delimiter: 'legacy',
        use_parent_child: true,
      },
    });

    expect(result?.parent_child).toEqual({
      children_delimiter: '\\n',
      use_parent_child: false,
    });
    expect(result?.ext).not.toHaveProperty('parent_child');
  });
});
