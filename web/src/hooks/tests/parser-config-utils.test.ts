import {
  extractParserConfigExt,
  normalizeInboundParserConfig,
} from '../parser-config-utils';

describe('normalizeInboundParserConfig', () => {
  it('maps parent_child settings onto enable_children for the dataset form', () => {
    const result = normalizeInboundParserConfig({
      parent_child: {
        use_parent_child: true,
        children_delimiter: '****',
      },
    });

    expect(result).toMatchObject({
      enable_children: true,
      children_delimiter: '****',
    });
  });

  it('prefers flattened enable_children when parent_child is absent', () => {
    const result = normalizeInboundParserConfig({
      enable_children: true,
      children_delimiter: '****',
    });

    expect(result).toMatchObject({
      enable_children: true,
      children_delimiter: '****',
    });
  });
});

describe('extractParserConfigExt', () => {
  it('serializes parent-child chunking for dataset update APIs', () => {
    const result = extractParserConfigExt({
      enable_children: true,
      children_delimiter: '****',
    });

    expect(result?.parent_child).toEqual({
      children_delimiter: '****',
      use_parent_child: true,
    });
  });

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
});
