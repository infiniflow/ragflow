import api from '@/utils/api';
import { registerNextServer } from '@/utils/register-server';

const {
  listDataflow,
  removeDataflow,
  fetchDataflow,
  runDataflow,
  setDataflow,
} = api;

const methods = {
  listDataflow: {
    url: listDataflow,
    method: 'get',
  },
  removeDataflow: {
    url: removeDataflow,
    method: 'post',
  },
  fetchDataflow: {
    url: fetchDataflow,
    method: 'get',
  },
  runDataflow: {
    url: runDataflow,
    method: 'post',
  },
  setDataflow: {
    url: setDataflow,
    method: 'post',
  },
} as const;

const dataflowService = registerNextServer<keyof typeof methods>(methods);

export default dataflowService;
