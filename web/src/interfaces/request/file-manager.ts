import { IPaginationRequestBody } from './base';

export interface IFileListRequestBody extends IPaginationRequestBody {
  parent_id?: string; // folder id
}

interface BaseRequestBody {
  parentId: string;
}

export interface IConnectRequestBody {
  fileIds: string[];
  kbIds: string[];
}
