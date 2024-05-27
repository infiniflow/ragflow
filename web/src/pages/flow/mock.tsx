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
