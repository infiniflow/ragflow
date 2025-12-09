// JSONEditor configuration options interface see: https://github.com/josdejong/jsoneditor/blob/master/docs/api.md
export interface JsonEditorOptions {
  /**
   * Editor mode. Available values: 'tree' (default), 'view', 'form', 'text', and 'code'.
   */
  mode?: 'tree' | 'view' | 'form' | 'text' | 'code';

  /**
   * Array of available modes
   */
  modes?: Array<'tree' | 'view' | 'form' | 'text' | 'code'>;

  /**
   * Field name for the root node. Only applicable for modes 'tree', 'view', and 'form'
   */
  name?: string;

  /**
   * Theme for the editor
   */
  theme?: string;

  /**
   * Enable history (undo/redo). True by default. Only applicable for modes 'tree', 'view', and 'form'
   */
  history?: boolean;

  /**
   * Enable search box. True by default. Only applicable for modes 'tree', 'view', and 'form'
   */
  search?: boolean;

  /**
   * Main menu bar visibility
   */
  mainMenuBar?: boolean;

  /**
   * Navigation bar visibility
   */
  navigationBar?: boolean;

  /**
   * Status bar visibility
   */
  statusBar?: boolean;

  /**
   * If true, object keys are sorted before display. false by default.
   */
  sortObjectKeys?: boolean;

  /**
   * Enable transform functionality
   */
  enableTransform?: boolean;

  /**
   * Enable sort functionality
   */
  enableSort?: boolean;

  /**
   * Limit dragging functionality
   */
  limitDragging?: boolean;

  /**
   * A JSON schema object
   */
  schema?: any;

  /**
   * Schemas that are referenced using the `$ref` property from the JSON schema
   */
  schemaRefs?: Record<string, any>;

  /**
   * Array of template objects
   */
  templates?: Array<{
    text: string;
    title?: string;
    className?: string;
    field?: string;
    value: any;
  }>;

  /**
   * Ace editor instance
   */
  ace?: any;

  /**
   * An instance of Ajv JSON schema validator
   */
  ajv?: any;

  /**
   * Switch to enable/disable autocomplete
   */
  autocomplete?: {
    confirmKey?: string | string[];
    caseSensitive?: boolean;
    getOptions?: (
      text: string,
      path: Array<string | number>,
      input: string,
      editor: any,
    ) => string[] | Promise<string[]> | null;
  };

  /**
   * Number of indentation spaces. 4 by default. Only applicable for modes 'text' and 'code'
   */
  indentation?: number;

  /**
   * Available languages
   */
  languages?: string[];

  /**
   * Language of the editor
   */
  language?: string;

  /**
   * Callback method, triggered on change of contents. Does not pass the contents itself.
   * See also onChangeJSON and onChangeText.
   */
  onChange?: () => void;

  /**
   * Callback method, triggered in modes on change of contents, passing the changed contents as JSON.
   * Only applicable for modes 'tree', 'view', and 'form'.
   */
  onChangeJSON?: (json: any) => void;

  /**
   * Callback method, triggered in modes on change of contents, passing the changed contents as stringified JSON.
   */
  onChangeText?: (text: string) => void;

  /**
   * Callback method, triggered when an error occurs
   */
  onError?: (error: Error) => void;

  /**
   * Callback method, triggered when node is expanded
   */
  onExpand?: (node: any) => void;

  /**
   * Callback method, triggered when node is collapsed
   */
  onCollapse?: (node: any) => void;

  /**
   * Callback method, determines if a node is editable
   */
  onEditable?: (node: any) => boolean | { field: boolean; value: boolean };

  /**
   * Callback method, triggered when an event occurs in a JSON field or value.
   * Only applicable for modes 'form', 'tree' and 'view'
   */
  onEvent?: (node: any, event: Event) => void;

  /**
   * Callback method, triggered when the editor comes into focus, passing an object {type, target}.
   * Applicable for all modes
   */
  onFocus?: (node: any) => void;

  /**
   * Callback method, triggered when the editor goes out of focus, passing an object {type, target}.
   * Applicable for all modes
   */
  onBlur?: (node: any) => void;

  /**
   * Callback method, triggered when creating menu items
   */
  onCreateMenu?: (menuItems: any[], node: any) => any[];

  /**
   * Callback method, triggered on node selection change. Only applicable for modes 'tree', 'view', and 'form'
   */
  onSelectionChange?: (selection: any) => void;

  /**
   * Callback method, triggered on text selection change. Only applicable for modes 'text' and 'code'
   */
  onTextSelectionChange?: (selection: any) => void;

  /**
   * Callback method, triggered when a Node DOM is rendered. Function returns a css class name to be set on a node.
   * Only applicable for modes 'form', 'tree' and 'view'
   */
  onClassName?: (node: any) => string | undefined;

  /**
   * Callback method, triggered when validating nodes
   */
  onValidate?: (
    json: any,
  ) =>
    | Array<{ path: Array<string | number>; message: string }>
    | Promise<Array<{ path: Array<string | number>; message: string }>>;

  /**
   * Callback method, triggered when node name is determined
   */
  onNodeName?: (parentNode: any, childNode: any, name: string) => string;

  /**
   * Callback method, triggered when mode changes
   */
  onModeChange?: (newMode: string, oldMode: string) => void;

  /**
   * Color picker options
   */
  colorPicker?: boolean;

  /**
   * Callback method for color picker
   */
  onColorPicker?: (
    callback: (color: string) => void,
    parent: HTMLElement,
  ) => void;

  /**
   * If true, shows timestamp tag
   */
  timestampTag?: boolean;

  /**
   * Format for timestamps
   */
  timestampFormat?: string;

  /**
   * If true, Unicode characters are escaped. false by default.
   */
  escapeUnicode?: boolean;

  /**
   * Number of children allowed for a node in 'tree', 'view', or 'form' mode before
   * the "show more/show all" buttons appear. 100 by default.
   */
  maxVisibleChilds?: number;

  /**
   * Callback method for validation errors
   */
  onValidationError?: (
    errors: Array<{ path: Array<string | number>; message: string }>,
  ) => void;

  /**
   * Callback method for validation warnings
   */
  onValidationWarning?: (
    warnings: Array<{ path: Array<string | number>; message: string }>,
  ) => void;

  /**
   * The anchor element to apply an overlay and display the modals in a centered location. Defaults to document.body
   */
  modalAnchor?: HTMLElement | null;

  /**
   * Anchor element for popups
   */
  popupAnchor?: HTMLElement | null;

  /**
   * Function to create queries
   */
  createQuery?: () => void;

  /**
   * Function to execute queries
   */
  executeQuery?: () => void;

  /**
   * Query description
   */
  queryDescription?: string;

  /**
   * Allow schema suggestions
   */
  allowSchemaSuggestions?: boolean;

  /**
   * Show error table
   */
  showErrorTable?: boolean;

  /**
   * Validate current JSON object against the configured JSON schema
   * Must be implemented by tree mode and text mode
   */
  validate?: () => Promise<any[]>;

  /**
   * Refresh the rendered contents
   * Can be implemented by tree mode and text mode
   */
  refresh?: () => void;

  /**
   * Callback method triggered when schema changes
   */
  _onSchemaChange?: (schema: any, schemaRefs: any) => void;
}

export interface JsonEditorProps {
  // JSON data to be displayed in the editor
  value?: any;

  // Callback function triggered when the JSON data changes
  onChange?: (value: any) => void;

  // Height of the editor
  height?: string;

  // Additional CSS class names
  className?: string;

  // Configuration options for the JSONEditor
  options?: JsonEditorOptions;
}
