export interface ICategorizeItem {
  name: string;
  description?: string;
  examples?: { value: string }[];
  index: number;
}

export type ICategorizeItemResult = Record<
  string,
  Omit<ICategorizeItem, 'name' | 'examples'> & { examples: string[] }
>;

export interface ISwitchCondition {
  items: ISwitchItem[];
  logical_operator: string;
  to: string[];
}

export interface ISwitchItem {
  cpn_id: string;
  operator: string;
  value: string;
}

export interface ISwitchForm {
  conditions: ISwitchCondition[];
  end_cpn_ids: string[];
  no: string;
}
