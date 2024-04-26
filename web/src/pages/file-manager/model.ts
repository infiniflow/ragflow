import { paginationModel } from '@/base';
import { BaseState } from '@/interfaces/common';
import { IFile, IFolder } from '@/interfaces/database/file-manager';
import i18n from '@/locales/config';
import fileManagerService from '@/services/fileManagerService';
import { message } from 'antd';
import omit from 'lodash/omit';
import { DvaModel } from 'umi';

export interface FileManagerModelState extends BaseState {
  fileList: IFile[];
  parentFolderList: IFolder[];
}

const model: DvaModel<FileManagerModelState> = {
  namespace: 'fileManager',
  state: {
    fileList: [],
    parentFolderList: [],
    ...(paginationModel.state as BaseState),
  },
  reducers: {
    setFileList(state, { payload }) {
      return { ...state, fileList: payload };
    },
    setParentFolderList(state, { payload }) {
      return { ...state, parentFolderList: payload };
    },
    ...paginationModel.reducers,
  },
  effects: {
    *removeFile({ payload = {} }, { call, put }) {
      const { data } = yield call(fileManagerService.removeFile, {
        fileIds: payload.fileIds,
      });
      const { retcode } = data;
      if (retcode === 0) {
        message.success(i18n.t('message.deleted'));
        yield put({
          type: 'listFile',
          payload: { parentId: payload.parentId },
        });
      }
      return retcode;
    },
    *listFile({ payload = {} }, { call, put, select }) {
      const { searchString, pagination } = yield select(
        (state: any) => state.fileManager,
      );
      const { current, pageSize } = pagination;
      const { data } = yield call(fileManagerService.listFile, {
        ...payload,
        keywords: searchString.trim(),
        page: current,
        pageSize,
      });
      const { retcode, data: res } = data;
      if (retcode === 0 && Array.isArray(res.files)) {
        yield put({
          type: 'setFileList',
          payload: res.files,
        });
        yield put({
          type: 'setPagination',
          payload: { total: res.total },
        });
      }
    },
    *renameFile({ payload = {} }, { call, put }) {
      const { data } = yield call(
        fileManagerService.renameFile,
        omit(payload, ['parentId']),
      );
      if (data.retcode === 0) {
        message.success(i18n.t('message.renamed'));
        yield put({
          type: 'listFile',
          payload: { parentId: payload.parentId },
        });
      }
      return data.retcode;
    },
    *uploadFile({ payload = {} }, { call, put }) {
      const fileList = payload.file;
      const pathList = payload.path;
      const formData = new FormData();
      formData.append('parent_id', payload.parentId);
      // formData.append('file', payload.file);
      // formData.append('path', payload.path);
      fileList.forEach((file: any, index: number) => {
        formData.append('file', file);
        formData.append('path', pathList[index]);
      });
      const { data } = yield call(fileManagerService.uploadFile, formData);
      if (data.retcode === 0) {
        message.success(i18n.t('message.uploaded'));

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
        message.success(i18n.t('message.created'));

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
    *connectFileToKnowledge({ payload = {} }, { call, put }) {
      const { data } = yield call(
        fileManagerService.connectFileToKnowledge,
        omit(payload, 'parentId'),
      );
      if (data.retcode === 0) {
        message.success(i18n.t('message.operated'));
        yield put({
          type: 'listFile',
          payload: { parentId: payload.parentId },
        });
      }
      return data.retcode;
    },
  },
};

// const finalModel = modelExtend<DvaModel<FileManagerModelState & BaseState>>(
//   paginationModel,
//   model,
// );

// console.info(finalModel);

export default model;
