import { IFile } from '@/interfaces/database/file-manager';
import kbService from '@/services/fileManagerService';
import { DvaModel } from 'umi';

export interface FileManagerModelState {
  fileList: IFile[];
}

const model: DvaModel<FileManagerModelState> = {
  namespace: 'fileManager',
  state: { fileList: [] },
  reducers: {
    setFileList(state, { payload }) {
      return { ...state, fileList: payload };
    },
  },
  effects: {
    *removeFile({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.removeFile, payload);
      const { retcode } = data;
      if (retcode === 0) {
        yield put({
          type: 'listFile',
          payload: data.data?.files ?? [],
        });
      }
    },
    *listFile({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.listFile, payload);
      const { retcode, data: res } = data;

      if (retcode === 0 && Array.isArray(res.files)) {
        yield put({
          type: 'setFileList',
          payload: res.files,
        });
      }
    },
    *renameFile({ payload = {} }, { call, put }) {
      const { data } = yield call(kbService.renameFile, payload);
      if (data.retcode === 0) {
        yield put({ type: 'listFile' });
      }
      return data.retcode;
    },
  },
};
export default model;
