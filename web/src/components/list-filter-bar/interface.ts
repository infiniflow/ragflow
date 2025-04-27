export type FilterType = {
  id: string;
  label: string;
  count: number;
};

export type FilterCollection = {
  field: string;
  label: string;
  list: FilterType[];
};

export type FilterValue = Record<string, Array<string>>;

export type FilterChange = (value: FilterValue) => void;
