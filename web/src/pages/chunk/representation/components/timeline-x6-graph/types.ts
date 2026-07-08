import {
  type TimelineX6EdgeData,
  type TimelineX6NodeData,
} from '../../utils/adapters';

export interface TimelineNodeValue {
  id: string;
  name: string;
  source_chunk_ids?: string[];
}

export interface TimelineX6GraphProps {
  data: {
    nodes: TimelineX6NodeData[];
    edges: TimelineX6EdgeData[];
  };
  show?: boolean;
  onNodeClick?: (node: TimelineNodeValue) => void;
}
