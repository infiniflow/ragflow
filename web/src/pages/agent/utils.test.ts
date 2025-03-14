import fs from 'fs';
import path from 'path';
import customer_service from '../../../../graph/test/dsl_examples/customer_service.json';
import headhunter_zh from '../../../../graph/test/dsl_examples/headhunter_zh.json';
import interpreter from '../../../../graph/test/dsl_examples/interpreter.json';
import retrievalRelevantRewriteAndGenerate from '../../../../graph/test/dsl_examples/retrieval_relevant_rewrite_and_generate.json';
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

test('build nodes and edges from  headhunter_zh dsl', () => {
  const { edges, nodes } = buildNodesAndEdgesFromDSLComponents(
    headhunter_zh.components,
  );
  console.info('node length', nodes.length);
  console.info('edge length', edges.length);
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

test('build nodes and edges from customer_service dsl', () => {
  const { edges, nodes } = buildNodesAndEdgesFromDSLComponents(
    customer_service.components,
  );
  console.info('node length', nodes.length);
  console.info('edge length', edges.length);
  try {
    fs.writeFileSync(
      path.join(__dirname, 'customer_service.json'),
      JSON.stringify({ edges, nodes }, null, 4),
    );
    console.log('JSON data is saved.');
  } catch (error) {
    console.warn(error);
  }
  expect(nodes.length).toEqual(12);
});

test('build nodes and edges from interpreter dsl', () => {
  const { edges, nodes } = buildNodesAndEdgesFromDSLComponents(
    interpreter.components,
  );
  console.info('node length', nodes.length);
  console.info('edge length', edges.length);
  try {
    fs.writeFileSync(
      path.join(__dirname, 'interpreter.json'),
      JSON.stringify({ edges, nodes }, null, 4),
    );
    console.log('JSON data is saved.');
  } catch (error) {
    console.warn(error);
  }
  expect(nodes.length).toEqual(12);
});

test('build nodes and edges from chat bot dsl', () => {
  const { edges, nodes } = buildNodesAndEdgesFromDSLComponents(
    retrievalRelevantRewriteAndGenerate.components,
  );
  try {
    fs.writeFileSync(
      path.join(__dirname, 'retrieval_relevant_rewrite_and_generate.json'),
      JSON.stringify({ edges, nodes }, null, 4),
    );
    console.log('JSON data is saved.');
  } catch (error) {
    console.warn(error);
  }
  expect(nodes.length).toEqual(12);
});
