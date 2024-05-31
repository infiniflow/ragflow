export interface DSLComponentList {
  id: string;
  name: string;
}

export interface IOperatorForm {
  onValuesChange?(changedValues: any, values: any): void;
}
