export interface DatasetNavNode {
  name: string;
  description: string;
  doc_count: number;
  type: string;
  doc_id?: string;
  has_children: boolean;
  keywords?: string[];
  entities?: string[];
  graph_content?: string;
}

export interface DatasetNavList {
  total: number;
  items: DatasetNavNode[];
}
