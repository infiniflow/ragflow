import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const {
  getCanvas,
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
} = api;

const methods = {
  getCanvas: {
    url: getCanvas,
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
} as const;

const chatService = registerServer<keyof typeof methods>(methods, request);

export default chatService;
