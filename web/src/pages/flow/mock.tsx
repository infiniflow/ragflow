import {
  MergeCellsOutlined,
  RocketOutlined,
  SendOutlined,
} from '@ant-design/icons';

export const componentList = [
  { name: 'Begin', icon: <SendOutlined />, description: '' },
  { name: 'Retrieval', icon: <RocketOutlined />, description: '' },
  { name: 'Generate', icon: <MergeCellsOutlined />, description: '' },
];

export const dsl = {
  components: {
    begin: {
      obj: {
        component_name: 'Begin',
        params: {},
      },
      downstream: ['Answer:China'],
      upstream: [],
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
