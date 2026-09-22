import { Edge } from '@xyflow/react';
import {
  CanvasChecklistInputs,
  CanvasIssueType,
  collectCanvasIssues,
} from '../canvas-checklist';

const makeNode = (
  id: string,
  label: string,
  form: Record<string, any> = {},
  extra: Record<string, any> = {},
) =>
  ({
    id,
    data: { label, name: id, form },
    ...extra,
  }) as any;

const makeEdge = (source: string, target: string, sourceHandle?: string) =>
  ({ id: `${source}->${target}`, source, target, sourceHandle }) as Edge;

const collect = (
  overrides: Partial<CanvasChecklistInputs> & {
    nodes: CanvasChecklistInputs['nodes'];
  },
) =>
  collectCanvasIssues({
    edges: [],
    editedNodeFormIds: [],
    ...overrides,
  });

describe('collectCanvasIssues: orphan steps', () => {
  it('flags a node with no incident edges', () => {
    const issues = collect({
      nodes: [makeNode('Parser:p1', 'Parser', { setups: [{}] })],
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Parser:p1',
        type: CanvasIssueType.Orphan,
        messageKey: 'flow.issueNotConnected',
      }),
    );
  });

  it('treats any edge — including tool edges — as a connection', () => {
    const issues = collect({
      nodes: [
        makeNode('Agent:a1', 'Agent', { llm_id: 'model-1', tools: [] }),
        makeNode('Tool:t1', 'Tool'),
      ],
      edges: [makeEdge('Agent:a1', 'Tool:t1', 'tool')],
    });

    expect(
      issues.filter((x) => x.type === CanvasIssueType.Orphan),
    ).toHaveLength(0);
  });

  it.each([
    ['begin', 'Begin'],
    ['Note:n1', 'Note'],
    ['Placeholder:p1', 'Placeholder'],
    ['IterationItem:i1', 'IterationItem'],
    ['LoopItem:l1', 'LoopItem'],
    ['ExitLoop:e1', 'ExitLoop'],
  ])('exempts structural node %s', (id, label) => {
    const issues = collect({ nodes: [makeNode(id, label)] });

    expect(
      issues.some((x) => x.nodeId === id && x.type === CanvasIssueType.Orphan),
    ).toBe(false);
  });

  it('flags a pipeline File node without downstream', () => {
    const issues = collect({ nodes: [makeNode('File', 'File', {})] });

    expect(issues).toContainEqual(
      expect.objectContaining({ nodeId: 'File', type: CanvasIssueType.Orphan }),
    );
  });
});

describe('collectCanvasIssues: dangling variable references', () => {
  const retrievalNode = () =>
    makeNode('Retrieval:r1', 'Retrieval', {
      retrieval_from: 'dataset',
      dataset_ids: ['kb-1'],
      outputs: { formalized_content: { type: 'string' } },
    });

  it('accepts a reference to a connected upstream output', () => {
    const issues = collect({
      nodes: [
        retrievalNode(),
        makeNode('Message:m1', 'Message', {
          content: ['{Retrieval:r1@formalized_content}'],
        }),
      ],
      edges: [makeEdge('Retrieval:r1', 'Message:m1')],
    });

    expect(
      issues.filter((x) => x.type === CanvasIssueType.InvalidVariable),
    ).toHaveLength(0);
  });

  it('flags the reference once the connecting edge is removed', () => {
    const issues = collect({
      nodes: [
        retrievalNode(),
        makeNode('Message:m1', 'Message', {
          content: ['{Retrieval:r1@formalized_content}'],
        }),
      ],
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Message:m1',
        type: CanvasIssueType.InvalidVariable,
        messageKey: 'flow.issueVariableInvalid',
        messageParams: { variable: 'Retrieval:r1@formalized_content' },
      }),
    );
  });

  it('flags a reference whose target node was deleted', () => {
    const issues = collect({
      nodes: [
        makeNode('Message:m1', 'Message', {
          content: ['{Retrieval:gone@formalized_content}'],
        }),
      ],
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Message:m1',
        type: CanvasIssueType.InvalidVariable,
        messageParams: { variable: 'Retrieval:gone@formalized_content' },
      }),
    );
  });

  it('flags a reference whose output key no longer exists', () => {
    const renamed = retrievalNode();
    renamed.data.form.outputs = { json: { type: 'object' } };
    const issues = collect({
      nodes: [
        renamed,
        makeNode('Message:m1', 'Message', {
          content: ['{Retrieval:r1@formalized_content}'],
        }),
      ],
      edges: [makeEdge('Retrieval:r1', 'Message:m1')],
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Message:m1',
        type: CanvasIssueType.InvalidVariable,
      }),
    );
  });

  it('skips the output-key check for nodes declaring no outputs (Message)', () => {
    const issues = collect({
      nodes: [
        makeNode('Message:m1', 'Message', { content: ['hello'] }),
        makeNode('Message:m2', 'Message', {
          content: ['{Message:m1@content}'],
        }),
      ],
      edges: [makeEdge('Message:m1', 'Message:m2')],
    });

    expect(
      issues.filter((x) => x.type === CanvasIssueType.InvalidVariable),
    ).toHaveLength(0);
  });

  it('reports a duplicated dangling reference only once per node', () => {
    const issues = collect({
      nodes: [
        makeNode('Message:m1', 'Message', {
          content: ['{Retrieval:gone@x}', 'again {Retrieval:gone@x}'],
        }),
      ],
    });

    expect(
      issues.filter(
        (x) =>
          x.type === CanvasIssueType.InvalidVariable &&
          x.messageParams?.variable === 'Retrieval:gone@x',
      ),
    ).toHaveLength(1);
  });

  it('detects bare whole-string references such as switch cpn_id', () => {
    const issues = collect({
      nodes: [
        retrievalNode(),
        makeNode('Switch:s1', 'Switch', {
          conditions: [{ cpn_id: 'Retrieval:r1@formalized_content' }],
        }),
      ],
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Switch:s1',
        type: CanvasIssueType.InvalidVariable,
        messageParams: { variable: 'Retrieval:r1@formalized_content' },
      }),
    );
  });

  it('ignores brace-like text inside the Code code field', () => {
    const issues = collect({
      nodes: [
        makeNode('CodeExec:c1', 'CodeExec', {
          code: 'print("{Retrieval:gone@x}")',
          outputs: { result: { type: 'string' } },
        }),
      ],
    });

    expect(
      issues.filter((x) => x.type === CanvasIssueType.InvalidVariable),
    ).toHaveLength(0);
  });

  it('never scans password fields for references', () => {
    const issues = collect({
      nodes: [
        makeNode('ExeSQL:e1', 'ExeSQL', { password: 'user:p@ss' }),
        makeNode('Message:m1', 'Message', { content: ['hi'] }),
      ],
      edges: [makeEdge('ExeSQL:e1', 'Message:m1')],
    });

    expect(
      issues.filter((x) => x.type === CanvasIssueType.InvalidVariable),
    ).toHaveLength(0);
  });
});

describe('collectCanvasIssues: begin/sys/env references', () => {
  const beginNode = () =>
    makeNode('begin', 'Begin', { inputs: { query: { type: 'line' } } });

  it('accepts an existing begin input and flags a deleted one', () => {
    const issues = collect({
      nodes: [
        beginNode(),
        makeNode('Message:m1', 'Message', {
          content: ['{begin@query} {begin@removed}'],
        }),
      ],
      edges: [makeEdge('begin', 'Message:m1')],
    });

    const invalidVariables = issues
      .filter((x) => x.type === CanvasIssueType.InvalidVariable)
      .map((x) => x.messageParams?.variable);
    expect(invalidVariables).toEqual(['begin@removed']);
  });

  it('treats sys.* globals as always resolvable', () => {
    const issues = collect({
      nodes: [makeNode('Message:m1', 'Message', { content: ['{sys.query}'] })],
    });

    expect(
      issues.filter((x) => x.type === CanvasIssueType.InvalidVariable),
    ).toHaveLength(0);
  });

  it('flags a deleted conversation variable, but only once variables loaded', () => {
    const messageNode = makeNode('Message:m1', 'Message', {
      content: ['{env.token}'],
    });

    const valid = collect({ nodes: [messageNode], variables: { token: {} } });
    const invalid = collect({ nodes: [messageNode], variables: {} });
    const unsettled = collect({ nodes: [messageNode] });

    expect(
      valid.filter((x) => x.type === CanvasIssueType.InvalidVariable),
    ).toHaveLength(0);
    expect(invalid).toContainEqual(
      expect.objectContaining({
        type: CanvasIssueType.InvalidVariable,
        messageParams: { variable: 'env.token' },
      }),
    );
    expect(
      unsettled.filter((x) => x.type === CanvasIssueType.InvalidVariable),
    ).toHaveLength(0);
  });
});

describe('collectCanvasIssues: iteration/loop children references', () => {
  it('lets a Loop child reference the parent outputs', () => {
    const issues = collect({
      nodes: [
        makeNode('Loop:p1', 'Loop', { outputs: { content: {} } }),
        makeNode('LoopItem:s1', 'LoopItem', {}, { parentId: 'Loop:p1' }),
        makeNode(
          'Message:m1',
          'Message',
          { content: ['{Loop:p1@content}'] },
          { parentId: 'Loop:p1' },
        ),
      ],
      edges: [makeEdge('LoopItem:s1', 'Message:m1')],
    });

    expect(
      issues.filter((x) => x.type === CanvasIssueType.InvalidVariable),
    ).toHaveLength(0);
  });

  it('lets an Iteration child reference the parent upstream', () => {
    const issues = collect({
      nodes: [
        makeNode('File', 'File', {}),
        makeNode('Iteration:p1', 'Iteration', { outputs: {} }),
        makeNode(
          'IterationItem:s1',
          'IterationItem',
          {},
          { parentId: 'Iteration:p1' },
        ),
        makeNode(
          'Message:m1',
          'Message',
          { content: ['{File@result}'] },
          { parentId: 'Iteration:p1' },
        ),
      ],
      edges: [
        makeEdge('File', 'Iteration:p1'),
        makeEdge('IterationItem:s1', 'Message:m1'),
      ],
    });

    expect(
      issues.filter((x) => x.type === CanvasIssueType.InvalidVariable),
    ).toHaveLength(0);
  });
});

describe('collectCanvasIssues: missing required fields', () => {
  it('flags an empty Message', () => {
    const issues = collect({
      nodes: [makeNode('Message:m1', 'Message', { content: [''] })],
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Message:m1',
        type: CanvasIssueType.MissingRequired,
        messageKey: 'flow.messageMsg',
      }),
    );
  });

  it('flags a Retrieval node without dataset or memory binding', () => {
    const datasetIssues = collect({
      nodes: [
        makeNode('Retrieval:r1', 'Retrieval', {
          retrieval_from: 'dataset',
          dataset_ids: [],
        }),
      ],
    });
    const memoryIssues = collect({
      nodes: [
        makeNode('Retrieval:r2', 'Retrieval', {
          retrieval_from: 'memory',
          memory_ids: [],
        }),
      ],
    });

    expect(datasetIssues).toContainEqual(
      expect.objectContaining({ messageKey: 'flow.retrievalDatasetMissing' }),
    );
    expect(memoryIssues).toContainEqual(
      expect.objectContaining({ messageKey: 'flow.retrievalMemoryMissing' }),
    );
  });

  it('flags an Agent node without a model', () => {
    const issues = collect({
      nodes: [makeNode('Agent:a1', 'Agent', { llm_id: '' })],
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Agent:a1',
        type: CanvasIssueType.MissingRequired,
        messageKey: 'flow.agentModelMissing',
      }),
    );
  });

  it('flags an invalid Parser form only after the node was edited', () => {
    const parserNode = makeNode('Parser:p1', 'Parser', { setups: [] });
    const connected = [makeEdge('File', 'Parser:p1')];
    const fileNode = makeNode('File', 'File', {});

    const untouched = collect({
      nodes: [parserNode, fileNode],
      edges: connected,
    });
    const edited = collect({
      nodes: [parserNode, fileNode],
      edges: connected,
      editedNodeFormIds: ['Parser:p1'],
    });

    expect(untouched.some((x) => x.messageKey === 'flow.nodeFormInvalid')).toBe(
      false,
    );
    expect(edited).toContainEqual(
      expect.objectContaining({
        nodeId: 'Parser:p1',
        type: CanvasIssueType.MissingRequired,
        messageKey: 'flow.nodeFormInvalid',
      }),
    );
  });
});

describe('collectCanvasIssues: stale resources', () => {
  it('flags a Message memory that no longer exists', () => {
    const issues = collect({
      nodes: [
        makeNode('Message:m1', 'Message', {
          content: ['hi'],
          memory_ids: ['mem-gone'],
        }),
      ],
      memoryIds: new Set(['mem-1']),
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Message:m1',
        type: CanvasIssueType.InvalidVariable,
        messageKey: 'flow.memoryUnavailable',
      }),
    );
  });

  it('skips the memory check while the memory list is unsettled', () => {
    const issues = collect({
      nodes: [
        makeNode('Message:m1', 'Message', {
          content: ['hi'],
          memory_ids: ['mem-gone'],
        }),
      ],
    });

    expect(
      issues.filter((x) => x.messageKey === 'flow.memoryUnavailable'),
    ).toHaveLength(0);
  });

  it('flags a deleted model on any node carrying llm_id', () => {
    const issues = collect({
      nodes: [
        makeNode('Categorize:c1', 'Categorize', { llm_id: 'deleted-model' }),
      ],
      modelValidIds: new Set(['other-model']),
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Categorize:c1',
        type: CanvasIssueType.InvalidVariable,
        messageKey: 'common.modelUnavailable',
      }),
    );
  });

  it('flags a stale dataset on a Retrieval node', () => {
    const issues = collect({
      nodes: [
        makeNode('Retrieval:r1', 'Retrieval', {
          retrieval_from: 'dataset',
          dataset_ids: ['kb-stale'],
        }),
      ],
      staleDatasetIds: new Set(['kb-stale']),
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Retrieval:r1',
        type: CanvasIssueType.InvalidVariable,
        messageKey: 'chat.datasetUnavailable',
      }),
    );
  });
});

describe('collectCanvasIssues: agent-embedded retrieval tools', () => {
  const agentWithTool = () =>
    makeNode('Agent:a1', 'Agent', {
      llm_id: 'model-1',
      tools: [
        {
          component_name: 'Retrieval',
          id: 'tool-1',
          name: 'Docs',
          params: { retrieval_from: 'dataset', dataset_ids: [] },
        },
      ],
    });

  it('attributes the issue to the Tool canvas node when it exists', () => {
    const issues = collect({
      nodes: [agentWithTool(), makeNode('Tool:t1', 'Tool')],
      edges: [makeEdge('Agent:a1', 'Tool:t1', 'tool')],
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Tool:t1',
        toolId: 'tool-1',
        // Names the concrete tool under the concrete agent, not the Tool
        // node's own label ('flow.tool' on legacy canvases).
        nodeName: 'Agent:a1 / Docs',
        operatorLabel: 'Retrieval',
        type: CanvasIssueType.MissingRequired,
        messageKey: 'flow.retrievalDatasetMissing',
      }),
    );
  });

  it('falls back to the agent node when there is no Tool canvas node', () => {
    const issues = collect({ nodes: [agentWithTool()] });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Agent:a1',
        nodeName: 'Agent:a1 / Docs',
        type: CanvasIssueType.MissingRequired,
        messageKey: 'flow.retrievalDatasetMissing',
      }),
    );
  });

  it('distinguishes multiple retrieval tools of the same agent', () => {
    const agent = makeNode('Agent:a1', 'Agent', {
      llm_id: 'model-1',
      tools: [
        {
          component_name: 'Retrieval',
          id: 'tool-1',
          name: 'DocsA',
          params: { retrieval_from: 'dataset', dataset_ids: [] },
        },
        {
          component_name: 'Retrieval',
          id: 'tool-2',
          name: 'DocsB',
          params: { retrieval_from: 'memory', memory_ids: [] },
        },
      ],
    });

    const issues = collect({
      nodes: [agent, makeNode('Tool:t1', 'Tool')],
      edges: [makeEdge('Agent:a1', 'Tool:t1', 'tool')],
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        toolId: 'tool-1',
        nodeName: 'Agent:a1 / DocsA',
        messageKey: 'flow.retrievalDatasetMissing',
      }),
    );
    expect(issues).toContainEqual(
      expect.objectContaining({
        toolId: 'tool-2',
        nodeName: 'Agent:a1 / DocsB',
        messageKey: 'flow.retrievalMemoryMissing',
      }),
    );
  });

  it('checks stale datasets and memories inside tool params', () => {
    const agent = makeNode('Agent:a1', 'Agent', {
      llm_id: 'model-1',
      tools: [
        {
          component_name: 'Retrieval',
          id: 'tool-1',
          params: { retrieval_from: 'memory', memory_ids: ['mem-gone'] },
        },
      ],
    });

    const issues = collect({
      nodes: [agent],
      memoryIds: new Set(['mem-1']),
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Agent:a1',
        type: CanvasIssueType.InvalidVariable,
        messageKey: 'flow.memoryUnavailable',
      }),
    );
  });
});

describe('collectCanvasIssues: compiler template group', () => {
  it('flags a Compiler node with an empty template group', () => {
    const issues = collect({
      nodes: [
        makeNode('Compiler:c1', 'Compiler', {
          compilation_template_group_id: '',
        }),
      ],
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Compiler:c1',
        type: CanvasIssueType.MissingRequired,
        messageKey: 'knowledgeConfiguration.compilationTemplateRequired',
      }),
    );
  });

  it('flags a template group the current user cannot resolve', () => {
    const issues = collect({
      nodes: [
        makeNode('Compiler:c1', 'Compiler', {
          compilation_template_group_id: 'g-gone',
        }),
      ],
      compilationTemplateGroupIds: new Set(['g-1']),
    });

    expect(issues).toContainEqual(
      expect.objectContaining({
        nodeId: 'Compiler:c1',
        type: CanvasIssueType.InvalidVariable,
        messageKey: 'knowledgeConfiguration.compilationTemplateUnavailable',
      }),
    );
  });

  it('skips the availability check while the group list is unsettled', () => {
    const issues = collect({
      nodes: [
        makeNode('Compiler:c1', 'Compiler', {
          compilation_template_group_id: 'g-gone',
        }),
      ],
    });

    expect(
      issues.filter(
        (x) =>
          x.messageKey ===
          'knowledgeConfiguration.compilationTemplateUnavailable',
      ),
    ).toHaveLength(0);
  });

  it('accepts a resolvable template group', () => {
    const issues = collect({
      nodes: [
        makeNode('Compiler:c1', 'Compiler', {
          compilation_template_group_id: 'g-1',
        }),
      ],
      compilationTemplateGroupIds: new Set(['g-1']),
    });

    expect(
      issues.filter(
        (x) =>
          x.messageKey ===
            'knowledgeConfiguration.compilationTemplateUnavailable' ||
          x.messageKey === 'knowledgeConfiguration.compilationTemplateRequired',
      ),
    ).toHaveLength(0);
  });

  it('ignores the same form key on non-Compiler nodes', () => {
    const issues = collect({
      nodes: [
        makeNode('Categorize:c1', 'Categorize', {
          compilation_template_group_id: '',
        }),
      ],
    });

    expect(
      issues.filter(
        (x) =>
          x.messageKey === 'knowledgeConfiguration.compilationTemplateRequired',
      ),
    ).toHaveLength(0);
  });
});
