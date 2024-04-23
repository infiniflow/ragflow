export interface IFile {
  create_date: string;
  create_time: number;
  created_by: string;
  id: string;
  kb_ids: string[];
  location: string;
  name: string;
  parent_id: string;
  size: number;
  tenant_id: string;
  type: string;
  update_date: string;
  update_time: number;
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
}
