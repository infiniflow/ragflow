type Nullable<T> = T | null;

declare module '*.md' {
  const content: string;
  export default content;
}

declare module 'jsoneditor' {
  const JSONEditor: any;
  export default JSONEditor;
  export = JSONEditor;
}
