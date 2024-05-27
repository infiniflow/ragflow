import { dsl } from './mock';
import { buildNodesAndEdgesFromDSLComponents } from './utils';

test('buildNodesAndEdgesFromDSLComponents', () => {
  const { edges, nodes } = buildNodesAndEdgesFromDSLComponents(dsl.components);

  expect(nodes.length).toEqual(4);
});
