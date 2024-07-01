import fs from 'fs';
import path from 'path';
import headhunter_zh from '../../../../graph/test/dsl_examples/headhunter_zh.json';
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

test('build nodes and edges from dsl', () => {
  const { edges, nodes } = buildNodesAndEdgesFromDSLComponents(
    headhunter_zh.components,
  );
  try {
    fs.writeFileSync(
      path.join(__dirname, 'headhunter_zh.json'),
      JSON.stringify({ edges, nodes }, null, 4),
    );
    console.log('JSON data is saved.');
  } catch (error) {
    console.warn(error);
  }
  expect(nodes.length).toEqual(12);
});
