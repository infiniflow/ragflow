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
