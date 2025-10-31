import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import Editor, { type BeforeMount, type OnMount } from '@monaco-editor/react';
import { AlertCircle, Check, Loader2 } from 'lucide-react';
import type * as Monaco from 'monaco-editor';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useMonacoTheme } from '../../hooks/use-monaco-theme';
import { formatTranslation, useTranslation } from '../../hooks/use-translation';
import type { JSONSchema } from '../../types/json-schema';
import {
  validateJson,
  type ValidationResult,
} from '../../utils/json-validator';

/** @public */
export interface JsonValidatorProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  schema: JSONSchema;
}

/** @public */
export function JsonValidator({
  open,
  onOpenChange,
  schema,
}: JsonValidatorProps) {
  const t = useTranslation();
  const [jsonInput, setJsonInput] = useState('');
  const [validationResult, setValidationResult] =
    useState<ValidationResult | null>(null);
  const editorRef = useRef<Parameters<OnMount>[0] | null>(null);
  const debounceTimerRef = useRef<number | null>(null);
  const monacoRef = useRef<typeof Monaco | null>(null);
  const schemaMonacoRef = useRef<typeof Monaco | null>(null);
  const {
    currentTheme,
    defineMonacoThemes,
    configureJsonDefaults,
    defaultEditorOptions,
  } = useMonacoTheme();

  const validateJsonAgainstSchema = useCallback(() => {
    if (!jsonInput.trim()) {
      setValidationResult(null);
      return;
    }

    const result = validateJson(jsonInput, schema);
    setValidationResult(result);
  }, [jsonInput, schema]);

  useEffect(() => {
    if (debounceTimerRef.current) {
      clearTimeout(debounceTimerRef.current);
    }

    debounceTimerRef.current = setTimeout(() => {
      validateJsonAgainstSchema();
    }, 500);

    return () => {
      if (debounceTimerRef.current) {
        clearTimeout(debounceTimerRef.current);
      }
    };
  }, [validateJsonAgainstSchema]);

  const handleJsonEditorBeforeMount: BeforeMount = (monaco) => {
    monacoRef.current = monaco;
    defineMonacoThemes(monaco);
    configureJsonDefaults(monaco, schema);
  };

  const handleSchemaEditorBeforeMount: BeforeMount = (monaco) => {
    schemaMonacoRef.current = monaco;
    defineMonacoThemes(monaco);
  };

  const handleEditorDidMount: OnMount = (editor) => {
    editorRef.current = editor;
    editor.focus();
  };

  const handleEditorChange = (value: string | undefined) => {
    setJsonInput(value || '');
  };

  const goToError = (line: number, column: number) => {
    if (editorRef.current) {
      editorRef.current.revealLineInCenter(line);
      editorRef.current.setPosition({ lineNumber: line, column: column });
      editorRef.current.focus();
    }
  };

  // Create a modified version of defaultEditorOptions for the editor
  const editorOptions = {
    ...defaultEditorOptions,
    readOnly: false,
  };

  // Create read-only options for the schema viewer
  const schemaViewerOptions = {
    ...defaultEditorOptions,
    readOnly: true,
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-5xl max-h-[700px] flex flex-col jsonjoy">
        <DialogHeader>
          <DialogTitle>{t.validatorTitle}</DialogTitle>
          <DialogDescription>{t.validatorDescription}</DialogDescription>
        </DialogHeader>
        <div className="flex-1 flex flex-col md:flex-row gap-4 py-4 overflow-hidden h-[600px]">
          <div className="flex-1 flex flex-col h-full">
            <div className="text-sm font-medium mb-2">{t.validatorContent}</div>
            <div className="border rounded-md flex-1 h-full">
              <Editor
                height="600px"
                defaultLanguage="json"
                value={jsonInput}
                onChange={handleEditorChange}
                beforeMount={handleJsonEditorBeforeMount}
                onMount={handleEditorDidMount}
                loading={
                  <div className="flex items-center justify-center h-full w-full bg-secondary/30">
                    <Loader2 className="h-6 w-6 animate-spin" />
                  </div>
                }
                options={editorOptions}
                theme={currentTheme}
              />
            </div>
          </div>

          <div className="flex-1 flex flex-col h-full">
            <div className="text-sm font-medium mb-2">
              {t.validatorCurrentSchema}
            </div>
            <div className="border rounded-md flex-1 h-full">
              <Editor
                height="600px"
                defaultLanguage="json"
                value={JSON.stringify(schema, null, 2)}
                beforeMount={handleSchemaEditorBeforeMount}
                loading={
                  <div className="flex items-center justify-center h-full w-full bg-secondary/30">
                    <Loader2 className="h-6 w-6 animate-spin" />
                  </div>
                }
                options={schemaViewerOptions}
                theme={currentTheme}
              />
            </div>
          </div>
        </div>

        {validationResult && (
          <div
            className={`rounded-md p-4 ${validationResult.valid ? 'bg-green-50 border border-green-200' : 'bg-red-50 border border-red-200'} transition-all duration-300 ease-in-out`}
          >
            <div className="flex items-center">
              {validationResult.valid ? (
                <>
                  <Check className="h-5 w-5 text-green-500 mr-2" />
                  <p className="text-green-700 font-medium">
                    {t.validatorValid}
                  </p>
                </>
              ) : (
                <>
                  <AlertCircle className="h-5 w-5 text-red-500 mr-2" />
                  <p className="text-red-700 font-medium">
                    {validationResult.errors.length === 1
                      ? validationResult.errors[0].path === '/'
                        ? t.validatorErrorInvalidSyntax
                        : t.validatorErrorSchemaValidation
                      : formatTranslation(t.validatorErrorCount, {
                          count: validationResult.errors.length,
                        })}
                  </p>
                </>
              )}
            </div>

            {!validationResult.valid &&
              validationResult.errors &&
              validationResult.errors.length > 0 && (
                <div className="mt-3 max-h-[200px] overflow-y-auto">
                  {validationResult.errors[0] && (
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-sm font-medium text-red-700">
                        {validationResult.errors[0].path === '/'
                          ? t.validatorErrorPathRoot
                          : validationResult.errors[0].path}
                      </span>
                      {validationResult.errors[0].line && (
                        <span className="text-xs bg-gray-100 px-2 py-1 rounded text-gray-600">
                          {validationResult.errors[0].column
                            ? formatTranslation(
                                t.validatorErrorLocationLineAndColumn,
                                {
                                  line: validationResult.errors[0].line,
                                  column: validationResult.errors[0].column,
                                },
                              )
                            : formatTranslation(
                                t.validatorErrorLocationLineOnly,
                                { line: validationResult.errors[0].line },
                              )}
                        </span>
                      )}
                    </div>
                  )}
                  <ul className="space-y-2">
                    {validationResult.errors.map((error, index) => (
                      <button
                        key={`error-${error.path}-${index}`}
                        type="button"
                        className="w-full text-left bg-white border border-red-100 rounded-md p-3 shadow-xs hover:shadow-md transition-shadow duration-200 cursor-pointer"
                        onClick={() =>
                          error.line &&
                          error.column &&
                          goToError(error.line, error.column)
                        }
                      >
                        <div className="flex items-start justify-between">
                          <div className="flex-1">
                            <p className="text-sm font-medium text-red-700">
                              {error.path === '/'
                                ? t.validatorErrorPathRoot
                                : error.path}
                            </p>
                            <p className="text-sm text-gray-600 mt-1">
                              {error.message}
                            </p>
                          </div>
                          {error.line && (
                            <div className="text-xs bg-gray-100 px-2 py-1 rounded text-gray-600">
                              {error.column
                                ? formatTranslation(
                                    t.validatorErrorLocationLineAndColumn,
                                    { line: error.line, column: error.column },
                                  )
                                : formatTranslation(
                                    t.validatorErrorLocationLineOnly,
                                    { line: error.line },
                                  )}
                            </div>
                          )}
                        </div>
                      </button>
                    ))}
                  </ul>
                </div>
              )}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
