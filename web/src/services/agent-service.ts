import api from '@/utils/api';
import { registerNextServer } from '@/utils/register-server';

const {
  getCanvasSSE,
  setCanvas,
  listCanvas,
  resetCanvas,
  removeCanvas,
  runCanvas,
  listTemplates,
  testDbConnect,
  getInputElements,
  debug,
  listCanvasTeam,
  settingCanvas,
  uploadCanvasFile,
  trace,
  inputForm,
  fetchVersionList,
  fetchVersion,
  fetchCanvas,
} = api;

const methods = {
  fetchCanvas: {
    url: fetchCanvas,
    method: 'get',
  },
  getCanvasSSE: {
    url: getCanvasSSE,
    method: 'get',
  },
  setCanvas: {
    url: setCanvas,
    method: 'post',
  },
  fetchVersionList: {
    url: fetchVersionList,
    method: 'get',
  },
  fetchVersion: {
    url: fetchVersion,
    method: 'get',
  },
  listCanvas: {
    url: listCanvas,
    method: 'get',
  },
  resetCanvas: {
    url: resetCanvas,
    method: 'post',
  },
  removeCanvas: {
    url: removeCanvas,
    method: 'post',
  },
  runCanvas: {
    url: runCanvas,
    method: 'post',
  },
  listTemplates: {
    url: listTemplates,
    method: 'get',
  },
  testDbConnect: {
    url: testDbConnect,
    method: 'post',
  },
  getInputElements: {
    url: getInputElements,
    method: 'get',
  },
  debugSingle: {
    url: debug,
    method: 'post',
  },
  listCanvasTeam: {
    url: listCanvasTeam,
    method: 'get',
  },
  settingCanvas: {
    url: settingCanvas,
    method: 'post',
  },
  uploadCanvasFile: {
    url: uploadCanvasFile,
    method: 'post',
  },
  trace: {
    url: trace,
    method: 'get',
  },
  inputForm: {
    url: inputForm,
    method: 'get',
  },
} as const;

const agentService = registerNextServer<keyof typeof methods>(methods);

export default agentService;
