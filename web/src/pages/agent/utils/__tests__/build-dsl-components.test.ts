// Regression tests for buildDslComponentsByGraph's per-operator param
// transforms. Focus: ManualChunker must serialize the form (rules /
// hierarchyGroup) into the backend params shape (levels / numeric hierarchy)
// — the same contract the Manual template's obj.params bakes in and
// manual.go consumes.

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
        chunk_token_cap: 512,
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
    // UI-only fields must not leak into the backend params.
    expect(params.groupRules).toBeUndefined();
    expect(params.hierarchyRules).toBeUndefined();
    expect(params.rules).toBeUndefined();
    expect(params.hierarchyHierarchy).toBeUndefined();
    expect(params.hierarchyGroup).toBeUndefined();
    expect(params.chunk_token_cap).toBeUndefined();
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
