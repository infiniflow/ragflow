export interface IFile {
  create_date: string;
  create_time: number;
  created_by: string;
  id: string;
  kbs_info: { kb_id: string; kb_name: string }[];
  location: string;
  name: string;
  parent_id: string;
  size: number;
  tenant_id: string;
  type: string;
  update_date: string;
  update_time: number;
  source_type: string;
  has_child_folder?: boolean;
}

export interface IFolder {
  create_date: string;
  create_time: number;
  created_by: string;
  id: string;
  location: string;
  name: string;
  parent_id: string;
  size: number;
  tenant_id: string;
  type: string;
  update_date: string;
  update_time: number;
  source_type: string;
}

export type IFetchFileListResult = {
  files: IFile[];
  parent_folder: IFolder;
  total: number;
};
