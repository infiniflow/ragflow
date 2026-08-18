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

  it('preserves top-level metadata, built_in_metadata and enable_metadata fields', () => {
    const customMetadata = [{ key: 'author', type: 'string' }];
    const builtInMetadata = [{ key: 'file_name', type: 'string' }];
    const result = extractParserConfigExt({
      chunk_token_num: 512,
      metadata: customMetadata,
      built_in_metadata: builtInMetadata,
      enable_metadata: true,
      llm_id: 'llm-123',
    });

    expect(result?.metadata).toEqual(customMetadata);
    expect(result?.built_in_metadata).toEqual(builtInMetadata);
    expect(result?.enable_metadata).toBe(true);
    expect(result?.llm_id).toBe('llm-123');
    expect(result?.ext).not.toHaveProperty('metadata');
    expect(result?.ext).not.toHaveProperty('built_in_metadata');
    expect(result?.ext).not.toHaveProperty('enable_metadata');
    expect(result?.ext).not.toHaveProperty('llm_id');
  });
});
