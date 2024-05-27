import { dsl } from './mock';
import { buildNodesAndEdgesFromDSLComponents } from './utils';

test('buildNodesAndEdgesFromDSLComponents', () => {
  const { edges, nodes } = buildNodesAndEdgesFromDSLComponents(dsl.components);

  expect(nodes.length).toEqual(4);
  expect(edges.length).toEqual(4);

  expect(edges).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        source: 'begin',
        target: 'Answer:China',
      }),
      expect.objectContaining({
        source: 'Answer:China',
        target: 'Retrieval:China',
      }),
      expect.objectContaining({
        source: 'Retrieval:China',
        target: 'Generate:China',
      }),
      expect.objectContaining({
        source: 'Generate:China',
        target: 'Answer:China',
      }),
    ]),
  );
});
