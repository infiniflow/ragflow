export interface DatasetSkillTreeNode {
  skill_kwd: string;
  md_with_weight?: string;
  children_kwd?: DatasetSkillTreeNode[];
}

export interface DatasetSkillTree {
  id: string;
  kb_id: string;
  doc_id: string;
  compile_kwd: 'skill_all';
  skill_with_weight: DatasetSkillTreeNode[];
}

export interface DatasetSkillPage {
  id?: string;
  kb_id?: string;
  doc_id?: string;
  compile_kwd?: 'skill';
  skill_kwd: string;
  depth_int?: number;
  children_kwd?: string[];
  source_doc_ids?: string[];
  md_with_weight: string;
}

export interface HasAnySkillResponse {
  has: boolean;
}
