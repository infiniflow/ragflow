import { IFile, IFolder } from '@/interfaces/database/file-manager';
import fileManagerService from '@/services/fileManagerService';
import omit from 'lodash/omit';
import { DvaModel } from 'umi';

export interface FileManagerModelState {
  fileList: IFile[];
  parentFolderList: IFolder[];
}

const model: DvaModel<FileManagerModelState> = {
  namespace: 'fileManager',
  state: { fileList: [], parentFolderList: [] },
  reducers: {
    setFileList(state, { payload }) {
      return { ...state, fileList: payload };
    },
    setParentFolderList(state, { payload }) {
      return { ...state, parentFolderList: payload };
    },
  },
  effects: {
    *removeFile({ payload = {} }, { call, put }) {
      const { data } = yield call(fileManagerService.removeFile, {
        fileIds: payload.fileIds,
      });
      const { retcode } = data;
      if (retcode === 0) {
        yield put({
          type: 'listFile',
          payload: { parentId: payload.parentId },
        });
      }
    },
    *listFile({ payload = {} }, { call, put }) {
      const { data } = yield call(fileManagerService.listFile, payload);
      const { retcode, data: res } = data;

      if (retcode === 0 && Array.isArray(res.files)) {
        yield put({
          type: 'setFileList',
          payload: res.files,
        });
      }
    },
    *renameFile({ payload = {} }, { call, put }) {
      const { data } = yield call(
        fileManagerService.renameFile,
        omit(payload, ['parentId']),
      );
      if (data.retcode === 0) {
        yield put({
          type: 'listFile',
          payload: { parentId: payload.parentId },
        });
      }
      return data.retcode;
    },
    *createFolder({ payload = {} }, { call, put }) {
      const { data } = yield call(fileManagerService.createFolder, payload);
      if (data.retcode === 0) {
        yield put({
          type: 'listFile',
          payload: { parentId: payload.parentId },
        });
      }
      return data.retcode;
    },
    *getAllParentFolder({ payload = {} }, { call, put }) {
      const { data } = yield call(
        fileManagerService.getAllParentFolder,
        payload,
      );
      if (data.retcode === 0) {
        yield put({
          type: 'setParentFolderList',
          payload: data.data?.parent_folders ?? [],
        });
      }
      return data.retcode;
    },
  },
};
export default model;
