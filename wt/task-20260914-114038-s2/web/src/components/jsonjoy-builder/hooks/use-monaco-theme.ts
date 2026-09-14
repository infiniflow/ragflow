import { useIsDarkTheme } from '@/components/theme-provider';
import type * as Monaco from 'monaco-editor';
import type { JSONSchema } from '../types/json-schema.js';

export interface MonacoEditorOptions {
  minimap?: { enabled: boolean };
  fontSize?: number;
  fontFamily?: string;
  lineNumbers?: 'on' | 'off';
  roundedSelection?: boolean;
  scrollBeyondLastLine?: boolean;
  readOnly?: boolean;
  automaticLayout?: boolean;
  formatOnPaste?: boolean;
  formatOnType?: boolean;
  tabSize?: number;
  insertSpaces?: boolean;
  detectIndentation?: boolean;
  folding?: boolean;
  foldingStrategy?: 'auto' | 'indentation';
  renderLineHighlight?: 'all' | 'line' | 'none' | 'gutter';
  matchBrackets?: 'always' | 'near' | 'never';
  autoClosingBrackets?:
    | 'always'
    | 'languageDefined'
    | 'beforeWhitespace'
    | 'never';
  autoClosingQuotes?:
    | 'always'
    | 'languageDefined'
    | 'beforeWhitespace'
    | 'never';
  guides?: {
    bracketPairs?: boolean;
    indentation?: boolean;
  };
}

export const defaultEditorOptions: MonacoEditorOptions = {
  minimap: { enabled: false },
  fontSize: 14,
  fontFamily: "var(--font-sans), 'SF Mono', Monaco, Menlo, Consolas, monospace",
  lineNumbers: 'on',
  roundedSelection: false,
  scrollBeyondLastLine: false,
  readOnly: false,
  automaticLayout: true,
  formatOnPaste: true,
  formatOnType: true,
  tabSize: 2,
  insertSpaces: true,
  detectIndentation: true,
  folding: true,
  foldingStrategy: 'indentation',
  renderLineHighlight: 'all',
  matchBrackets: 'always',
  autoClosingBrackets: 'always',
  autoClosingQuotes: 'always',
  guides: {
    bracketPairs: true,
    indentation: true,
  },
};

export function useMonacoTheme() {
  // const [isDarkTheme, setisDarkTheme] = useState(false);
  const isDarkTheme = useIsDarkTheme();

  // Check for dark mode by examining CSS variables
  // useEffect(() => {
  //   const checkDarkMode = () => {
  //     // Get the current background color value
  //     const backgroundColor = getComputedStyle(document.documentElement)
  //       .getPropertyValue('--background')
  //       .trim();

  //     // If the background color HSL has a low lightness value, it's likely dark mode
  //     const isDark =
  //       backgroundColor.includes('222.2') ||
  //       backgroundColor.includes('84% 4.9%');

  //     setisDarkTheme(isDark);
  //   };

  //   // Check initially
  //   checkDarkMode();

  //   // Set up a mutation observer to detect theme changes
  //   const observer = new MutationObserver(checkDarkMode);
  //   observer.observe(document.documentElement, {
  //     attributes: true,
  //     attributeFilter: ['class', 'style'],
  //   });

  //   return () => observer.disconnect();
  // }, []);

  const defineMonacoThemes = (monaco: typeof Monaco) => {
    // Define custom light theme that matches app colors
    monaco.editor.defineTheme('appLightTheme', {
      base: 'vs',
      inherit: true,
      rules: [
        // JSON syntax highlighting based on utils.ts type colors
        { token: 'string', foreground: '3B82F6' }, // text-blue-500
        { token: 'number', foreground: 'A855F7' }, // text-purple-500
        { token: 'keyword', foreground: '3B82F6' }, // text-blue-500
        { token: 'delimiter', foreground: '0F172A' }, // text-slate-900
        { token: 'keyword.json', foreground: 'A855F7' }, // text-purple-500
        { token: 'string.key.json', foreground: '2563EB' }, // text-blue-600
        { token: 'string.value.json', foreground: '3B82F6' }, // text-blue-500
        { token: 'boolean', foreground: '22C55E' }, // text-green-500
        { token: 'null', foreground: '64748B' }, // text-gray-500
      ],
      colors: {
        // Light theme colors (using hex values instead of CSS variables)
        'editor.background': '#f8fafc', // --background
        'editor.foreground': '#0f172a', // --foreground
        'editorCursor.foreground': '#0f172a', // --foreground
        'editor.lineHighlightBackground': '#f1f5f9', // --muted
        'editorLineNumber.foreground': '#64748b', // --muted-foreground
        'editor.selectionBackground': '#e2e8f0', // --accent
        'editor.inactiveSelectionBackground': '#e2e8f0', // --accent
        'editorIndentGuide.background': '#e2e8f0', // --border
        'editor.findMatchBackground': '#cbd5e1', // --accent
        'editor.findMatchHighlightBackground': '#cbd5e133', // --accent with opacity
      },
    });

    // Define custom dark theme that matches app colors
    monaco.editor.defineTheme('appDarkTheme', {
      base: 'vs-dark',
      inherit: true,
      rules: [
        // JSON syntax highlighting based on utils.ts type colors
        { token: 'string', foreground: '3B82F6' }, // text-blue-500
        { token: 'number', foreground: 'A855F7' }, // text-purple-500
        { token: 'keyword', foreground: '3B82F6' }, // text-blue-500
        { token: 'delimiter', foreground: 'F8FAFC' }, // text-slate-50
        { token: 'keyword.json', foreground: 'A855F7' }, // text-purple-500
        { token: 'string.key.json', foreground: '60A5FA' }, // text-blue-400
        { token: 'string.value.json', foreground: '3B82F6' }, // text-blue-500
        { token: 'boolean', foreground: '22C55E' }, // text-green-500
        { token: 'null', foreground: '94A3B8' }, // text-gray-400
      ],
      colors: {
        // Dark theme colors (using hex values instead of CSS variables)
        'editor.background': '#0f172a', // --background
        'editor.foreground': '#f8fafc', // --foreground
        'editorCursor.foreground': '#f8fafc', // --foreground
        'editor.lineHighlightBackground': '#1e293b', // --muted
        'editorLineNumber.foreground': '#64748b', // --muted-foreground
        'editor.selectionBackground': '#334155', // --accent
        'editor.inactiveSelectionBackground': '#334155', // --accent
        'editorIndentGuide.background': '#1e293b', // --border
        'editor.findMatchBackground': '#475569', // --accent
        'editor.findMatchHighlightBackground': '#47556933', // --accent with opacity
      },
    });
  };

  // Helper to configure JSON language validation
  const configureJsonDefaults = (
    monaco: typeof Monaco,
    schema?: JSONSchema,
  ) => {
    // Create a new diagnostics options object
    const diagnosticsOptions: Monaco.languages.json.DiagnosticsOptions = {
      validate: true,
      allowComments: false,
      schemaValidation: 'error',
      enableSchemaRequest: true,
      schemas: schema
        ? [
            {
              uri:
                typeof schema === 'object' && schema.$id
                  ? schema.$id
                  : 'https://jsonjoy-builder/schema',
              fileMatch: ['*'],
              schema,
            },
          ]
        : [
            {
              uri: 'http://json-schema.org/draft-07/schema',
              fileMatch: ['*'],
              schema: {
                $schema: 'http://json-schema.org/draft-07/schema',
                type: 'object',
                additionalProperties: true,
              },
            },
          ],
    };

    monaco.languages.json.jsonDefaults.setDiagnosticsOptions(
      diagnosticsOptions,
    );
  };

  return {
    isDarkTheme,
    currentTheme: isDarkTheme ? 'appDarkTheme' : 'appLightTheme',
    defineMonacoThemes,
    configureJsonDefaults,
    defaultEditorOptions,
  };
}
