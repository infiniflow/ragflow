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
  /**
   * Tree edge: the parent cluster's name ("root" for a depth-0 cluster). A
   * keyword search returns the hits together with the cluster path above them,
   * so the flat list nests into one tree per root cluster.
   */
  parent_kwd?: string;
  /** Keyword-search hit marker; the path rows returned with it are not hits. */
  matched?: boolean;
}

export interface DatasetNavList {
  /**
   * Cluster count: the depth-0 clusters when browsing, or the clusters the hits
   * landed in when searching. The nav tree header shows it in both modes, so a
   * search never re-purposes that heading as a document count.
   */
  total: number;
  items: DatasetNavNode[];
}
