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
});
