export type FilterType = {
  id: string;
  field?: string;
  label: string | JSX.Element;
  list?: FilterType[];
  value?: string | string[];
  count?: number;
  canSearch?: boolean;
};
export type FilterCollection = {
  field: string;
  label: string;
  list: FilterType[];
  canSearch?: boolean;
};
export type FilterValue = Record<
  string,
  Array<string> | Record<string, Array<string>>
>;
export type FilterChange = (value: FilterValue) => void;
