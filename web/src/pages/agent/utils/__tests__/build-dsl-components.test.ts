// Regression tests for buildDslComponentsByGraph's per-operator param
// transforms. Focus: ManualChunker must serialize the form (rules /
// hierarchyGroup) into the backend params shape (levels / numeric hierarchy)
// — the same contract the Manual template's obj.params bakes in and
// manual.go consumes. Each method's rules/hierarchy value/token cap are
// additionally kept under their own split keys (hierarchyRules/groupRules/
// hierarchyHierarchy/hierarchyGroup/hierarchyChunkTokenCap/
// groupChunkTokenCap) so the stored config stays independent of the active
// method; the backend reads only levels/hierarchy and ignores the rest.

import { Operator } from '../../constant';
import { buildDslComponentsByGraph } from '../../utils';

describe('buildDslComponentsByGraph ManualChunker params', () => {
  const manualChunkerNode = {
    id: 'ManualChunker:TestId',
    type: 'chunkerNode',
    position: { x: 0, y: 0 },
    data: {
      label: Operator.ManualChunker,
      name: 'Manual Chunker',
      form: {
        method: 'group',
        hierarchyHierarchy: '0',
        hierarchyGroup: '3',
        hierarchyRules: [{ levels: [{ expression: '^#[^#]' }] }],
        groupRules: [
          { levels: [{ expression: '第[0-9]+章' }] },
          { levels: [{ expression: '第[0-9]+节' }] },
        ],
        // Legacy field baked into the Manual template's node form; must not
        // leak into the serialized params (levels already carries it).
        rules: [{ levels: [{ expression: '^#[^#]' }] }],
        include_heading_content: true,
        root_chunk_as_heading: true,
        // The manual chunker ignores the token cap, so neither the legacy
        // shared key nor the per-method split keys may leak into the params.
        chunk_token_cap: 512,
        hierarchyChunkTokenCap: 512,
        groupChunkTokenCap: 512,
        outputs: { chunks: { type: 'Array<Object>', value: [] } },
      },
    },
  } as any;

  it('converts the form shape into the backend params shape', () => {
    const components = buildDslComponentsByGraph(
      [manualChunkerNode],
      [],
      {},
    ) as any;

    const params = components['ManualChunker:TestId'].obj.params;

    expect(params.hierarchy).toBe(3);
    expect(params.levels).toEqual([['第[0-9]+章'], ['第[0-9]+节']]);
    // The per-method split fields persist alongside the shared
    // levels/hierarchy keys so Hierarchy and Group configs stay independent.
    expect(params.hierarchyRules).toEqual([
      { levels: [{ expression: '^#[^#]' }] },
    ]);
    expect(params.groupRules).toEqual([
      { levels: [{ expression: '第[0-9]+章' }] },
      { levels: [{ expression: '第[0-9]+节' }] },
    ]);
    expect(params.hierarchyHierarchy).toBe('0');
    expect(params.hierarchyGroup).toBe('3');
    // The legacy pre-split rules field must not leak (levels carries it).
    expect(params.rules).toBeUndefined();
    expect(params.chunk_token_cap).toBeUndefined();
    expect(params.hierarchyChunkTokenCap).toBeUndefined();
    expect(params.groupChunkTokenCap).toBeUndefined();
    // include_heading_content is stripped before the transform but the
    // transform unconditionally re-adds it as false — the backend
    // (manual.go) pins method=group and ignores it either way.
    expect(params.include_heading_content).toBe(false);
  });

  it('keeps wire-format fields such as outputs and method', () => {
    const components = buildDslComponentsByGraph(
      [manualChunkerNode],
      [],
      {},
    ) as any;

    const params = components['ManualChunker:TestId'].obj.params;

    expect(params.method).toBe('group');
    expect(params.outputs).toEqual({
      chunks: { type: 'Array<Object>', value: [] },
    });
  });
});
