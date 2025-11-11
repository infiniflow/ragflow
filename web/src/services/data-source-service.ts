import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const { dataSourceSet, dataSourceList } = api;
const methods = {
  dataSourceSet: {
    url: dataSourceSet,
    method: 'post',
  },
  dataSourceList: {
    url: dataSourceList,
    method: 'get',
  },
} as const;
const dataSourceService = registerServer<keyof typeof methods>(
  methods,
  request,
);

export const deleteDataSource = (id: string) =>
  request.post(api.dataSourceDel(id));
export const dataSourceResume = (id: string, data: { resume: boolean }) => {
  return request.put(api.dataSourceResume(id), { data });
};

export const dataSourceRebuild = (id: string, data: { kb_id: string }) => {
  return request.put(api.dataSourceRebuild(id), { data });
};

export const getDataSourceLogs = (id: string, params?: any) =>
  request.get(api.dataSourceLogs(id), { params });
export const featchDataSourceDetail = (id: string) =>
  request.get(api.dataSourceDetail(id));

export const startGoogleDriveWebAuth = (payload: { credentials: string }) =>
  request.post(api.googleDriveWebAuthStart, { data: payload });

export const pollGoogleDriveWebAuthResult = (payload: { flow_id: string }) =>
  request.post(api.googleDriveWebAuthResult, { data: payload });

export default dataSourceService;
