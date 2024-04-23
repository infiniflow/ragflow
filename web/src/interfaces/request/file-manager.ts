import { IPaginationRequestBody } from './base';

export interface IFileListRequestBody extends IPaginationRequestBody {
  parent_id?: string; // folder id
}
