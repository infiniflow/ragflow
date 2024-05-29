import {
  MergeCellsOutlined,
  RocketOutlined,
  SendOutlined,
} from '@ant-design/icons';
import { Position } from 'reactflow';

export const componentList = [
  { name: 'Begin', icon: <SendOutlined />, description: '' },
  { name: 'Retrieval', icon: <RocketOutlined />, description: '' },
  { name: 'Generate', icon: <MergeCellsOutlined />, description: '' },
];

export const initialNodes = [
  {
    sourcePosition: Position.Left,
    targetPosition: Position.Right,
    id: 'node-1',
    type: 'textUpdater',
    position: { x: 0, y: 0 },
    // position: { x: 400, y: 100 },
    data: { label: 123 },
  },
  {
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
    id: '1',
    data: { label: 'Hello' },
    position: { x: 0, y: 0 },
    // position: { x: 0, y: 50 },
    type: 'input',
  },
  {
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
    id: '2',
    data: { label: 'World' },
    position: { x: 0, y: 0 },
    // position: { x: 200, y: 50 },
  },
];

export const initialEdges = [
  { id: '1-2', source: '1', target: '2', label: 'to the', type: 'step' },
];

export const dsl = {
  graph: {
    nodes: [
      {
        id: 'begin',
        type: 'textUpdater',
        position: {
          x: 50,
          y: 200,
        },
        data: {
          label: 'Begin',
        },
        sourcePosition: 'left',
        targetPosition: 'right',
      },
      {
        id: 'Answer:China',
        type: 'textUpdater',
        position: {
          x: 150,
          y: 200,
        },
        data: {
          label: 'Answer',
        },
        sourcePosition: 'left',
        targetPosition: 'right',
      },
      {
        id: 'Retrieval:China',
        type: 'textUpdater',
        position: {
          x: 250,
          y: 200,
        },
        data: {
          label: 'Retrieval',
        },
        sourcePosition: 'left',
        targetPosition: 'right',
      },
      {
        id: 'Generate:China',
        type: 'textUpdater',
        position: {
          x: 100,
          y: 100,
        },
        data: {
          label: 'Generate',
        },
        sourcePosition: 'left',
        targetPosition: 'right',
      },
    ],
    edges: [
      {
        id: '7facb53d-65c9-43b3-ac55-339c445d3891',
        label: '',
        source: 'begin',
        target: 'Answer:China',
        markerEnd: {
          type: 'arrow',
        },
      },
      {
        id: '7ac83631-502d-410f-a6e7-bec6866a5e99',
        label: '',
        source: 'Generate:China',
        target: 'Answer:China',
        markerEnd: {
          type: 'arrow',
        },
      },
      {
        id: '0aaab297-5779-43ed-9281-2c4d3741566f',
        label: '',
        source: 'Answer:China',
        target: 'Retrieval:China',
        markerEnd: {
          type: 'arrow',
        },
      },
      {
        id: '3477f9f3-0a7d-400e-af96-a11ea7673183',
        label: '',
        source: 'Retrieval:China',
        target: 'Generate:China',
        markerEnd: {
          type: 'arrow',
        },
      },
    ],
  },
  components: {
    begin: {
      obj: {
        component_name: 'Begin',
        params: {},
      },
      downstream: ['Answer:China'], // other edge target is downstream, edge source is current node id
      upstream: [], // edge source is upstream, edge target is current node id
    },
    'Answer:China': {
      obj: {
        component_name: 'Answer',
        params: {},
      },
      downstream: ['Retrieval:China'],
      upstream: ['begin', 'Generate:China'],
    },
    'Retrieval:China': {
      obj: {
        component_name: 'Retrieval',
        params: {},
      },
      downstream: ['Generate:China'],
      upstream: ['Answer:China'],
    },
    'Generate:China': {
      obj: {
        component_name: 'Generate',
        params: {},
      },
      downstream: ['Answer:China'],
      upstream: ['Retrieval:China'],
    },
  },
  history: [],
  path: ['begin'],
  answer: [],
};
