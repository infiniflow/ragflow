export interface DatasetNavNode {
  name: string;
  description: string;
  doc_count: number;
  type: string;
  doc_id?: string; // only returned by the children endpoint
  has_children: boolean;
}

export interface DatasetNavList {
  total: number;
  items: DatasetNavNode[];
}
