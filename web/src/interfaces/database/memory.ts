export interface IMemory {
  avatar: null;
  description: null;
  id: string;
  memory_type: string[];
  name: string;
  nickname?: string;
  user_id?: string;
  operator_permission?: number;
  permissions: string;
  storage_type: string;
  tenant_id: string;
  embd_id: string;
  embd_name?: string;
  llm_id: string;
}
