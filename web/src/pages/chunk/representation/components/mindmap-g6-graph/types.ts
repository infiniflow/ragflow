import { type IStructureGraphTemplate } from '@/interfaces/database/document-structure';

export interface MindMapNodeValue {
  id: string;
  name: string;
  source_chunk_ids?: string[];
}

export interface MindMapG6GraphProps {
  template: IStructureGraphTemplate;
  show?: boolean;
  onNodeClick?: (node: MindMapNodeValue) => void;
}
