import { IPaginationRequestBody } from './base';

export interface IFileListRequestBody extends IPaginationRequestBody {
  parent_id?: string; // folder id
  tenant_id?: string;
}

export interface IConnectRequestBody {
  fileIds: string[];
  kbIds: string[];
}

export type ConnectFileToKnowledgeMode = 'add' | 'replace';
