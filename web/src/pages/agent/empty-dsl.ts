const FILE_ID = 'File';
const FILE_OPERATOR = 'File';
const INITIAL_PARSER_VALUES = {
  outputs: {
    markdown: { type: 'string', value: '' },
    text: { type: 'string', value: '' },
    html: { type: 'string', value: '' },
    json: { type: 'Array<object>', value: [] },
  },
  setups: [],
};

// Dataflow seed DSL. Kept separate from UI hooks so pure DSL helpers
// can import it without pulling React modules into tests/runtime.
export const DataflowEmptyDsl = {
  graph: {
    nodes: [
      {
        id: FILE_ID,
        type: 'beginNode',
        position: {
          x: 50,
          y: 200,
        },
        data: {
          label: FILE_OPERATOR,
          name: FILE_OPERATOR,
        },
        sourcePosition: 'left',
        targetPosition: 'right',
      },
      {
        data: {
          form: INITIAL_PARSER_VALUES,
          label: 'Parser',
          name: 'Parser_0',
        },
        dragging: false,
        id: 'Parser:HipSignsRhyme',
        measured: {
          height: 57,
          width: 200,
        },
        position: {
          x: 316.99524094206413,
          y: 195.39629819663406,
        },
        selected: true,
        sourcePosition: 'right',
        targetPosition: 'left',
        type: 'parserNode',
      },
    ],
    edges: [
      {
        id: 'xy-edge__Filestart-Parser:HipSignsRhymeend',
        source: FILE_ID,
        sourceHandle: 'start',
        target: 'Parser:HipSignsRhyme',
        targetHandle: 'end',
      },
    ],
  },
  components: {
    [FILE_OPERATOR]: {
      obj: {
        component_name: FILE_OPERATOR,
        params: {},
      },
      downstream: [],
      upstream: [],
    },
  },
  retrieval: [],
  history: [],
  path: [],
  globals: {},
  variables: [],
};
