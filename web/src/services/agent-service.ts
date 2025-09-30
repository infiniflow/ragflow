import { IAgentLogsRequest } from '@/interfaces/database/agent';
import api from '@/utils/api';
import { registerNextServer } from '@/utils/register-server';
import request from '@/utils/request';

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
  fetchAgentAvatar,
  fetchAgentLogs,
  fetchExternalAgentInputs,
  createSchedule,
  listSchedules,
  updateSchedule,
  toggleSchedule,
  deleteSchedule,
  getFrequencyOptions,
  getScheduleHistory,
  getScheduleStats,
  prompt,
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
  fetchAgentAvatar: {
    url: fetchAgentAvatar,
    method: 'get',
  },
  fetchAgentLogs: {
    url: fetchAgentLogs,
    method: 'get',
  },
  fetchExternalAgentInputs: {
    url: fetchExternalAgentInputs,
    method: 'get',
  },
  createSchedule: {
    url: createSchedule,
    method: 'post',
  },
  listSchedules: {
    url: listSchedules,
    method: 'get',
  },
  updateSchedule: {
    url: updateSchedule,
    method: 'post',
  },
  toggleSchedule: {
    url: toggleSchedule,
    method: 'post',
  },
  deleteSchedule: {
    url: deleteSchedule,
    method: 'delete',
  },
  getFrequencyOptions: {
    url: getFrequencyOptions,
    method: 'get',
  },
  getScheduleHistory: {
    url: getScheduleHistory,
    method: 'get',
  },
  getScheduleStats: {
    url: getScheduleStats,
    method: 'get',
  },
  fetchPrompt: {
    url: prompt,
    method: 'get',
  },
} as const;

const agentService = registerNextServer<keyof typeof methods>(methods);



export const toggleScheduleById = (data: any, id: string) => {
  return request.post(toggleSchedule(id), data);
};

export const deleteScheduleById = (data: any, id: string) => {
  return request.delete(deleteSchedule(id), data);
};

export const getScheduleHistoryById = (params: any, id: string) => {
  return request.get(getScheduleHistory(id), { params });
};

export const getScheduleStatsById = (params: any, id: string) => {
  return request.get(getScheduleStats(id), { params });
};

export const fetchTrace = (data: { canvas_id: string; message_id: string }) => {
  return request.get(methods.trace.url, { params: data });
};
export const fetchAgentLogsByCanvasId = (
  canvasId: string,
  params: IAgentLogsRequest,
) => {
  return request.get(methods.fetchAgentLogs.url(canvasId), { params: params });
};

export default agentService;
