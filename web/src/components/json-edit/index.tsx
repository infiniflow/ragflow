import React, { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import './css/cloud9_night.less';
import './css/index.less';
import { JsonEditorOptions, JsonEditorProps } from './interface';
const defaultConfig: JsonEditorOptions = {
  mode: 'code',
  modes: ['tree', 'code'],
  history: false,
  search: false,
  mainMenuBar: false,
  navigationBar: false,
  enableSort: false,
  enableTransform: false,
  indentation: 2,
};
const JsonEditor: React.FC<JsonEditorProps> = ({
  value,
  onChange,
  height = '400px',
  className = '',
  options = {},
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<any>(null);
  const { i18n } = useTranslation();
  const currentLanguageRef = useRef<string>(i18n.language);

  useEffect(() => {
    if (typeof window !== 'undefined') {
      const JSONEditor = require('jsoneditor');
      import('jsoneditor/dist/jsoneditor.min.css');

      if (containerRef.current) {
        // Default configuration options
        const defaultOptions: JsonEditorOptions = {
          ...defaultConfig,
          language: i18n.language === 'zh' ? 'zh-CN' : 'en',
          onChange: () => {
            if (editorRef.current && onChange) {
              try {
                const updatedJson = editorRef.current.get();
                onChange(updatedJson);
              } catch (err) {
                // Do not trigger onChange when parsing error occurs
                console.error(err);
              }
            }
          },
          ...options, // Merge user provided options with defaults
        };

        editorRef.current = new JSONEditor(
          containerRef.current,
          defaultOptions,
        );

        if (value) {
          editorRef.current.set(value);
        }
      }
    }

    return () => {
      if (editorRef.current) {
        if (typeof editorRef.current.destroy === 'function') {
          editorRef.current.destroy();
        }
        editorRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    // Update language when i18n language changes
    // Since JSONEditor doesn't have a setOptions method, we need to recreate the editor
    if (editorRef.current && currentLanguageRef.current !== i18n.language) {
      currentLanguageRef.current = i18n.language;

      // Save current data
      let currentData;
      try {
        currentData = editorRef.current.get();
      } catch (e) {
        // If there's an error getting data, use the passed value or empty object
        currentData = value || {};
      }

      // Destroy the current editor
      if (typeof editorRef.current.destroy === 'function') {
        editorRef.current.destroy();
      }

      // Recreate the editor with new language
      const JSONEditor = require('jsoneditor');

      const newOptions: JsonEditorOptions = {
        ...defaultConfig,
        language: i18n.language === 'zh' ? 'zh-CN' : 'en',
        onChange: () => {
          if (editorRef.current && onChange) {
            try {
              const updatedJson = editorRef.current.get();
              onChange(updatedJson);
            } catch (err) {
              // Do not trigger onChange when parsing error occurs
            }
          }
        },
        ...options, // Merge user provided options with defaults
      };

      editorRef.current = new JSONEditor(containerRef.current, newOptions);
      editorRef.current.set(currentData);
    }
  }, [i18n.language, value, onChange, options]);

  useEffect(() => {
    if (editorRef.current && value !== undefined) {
      try {
        // Only update the editor when the value actually changes
        const currentJson = editorRef.current.get();
        if (JSON.stringify(currentJson) !== JSON.stringify(value)) {
          editorRef.current.set(value);
        }
      } catch (err) {
        // Skip update if there is a syntax error in the current editor
        editorRef.current.set(value);
      }
    }
  }, [value]);

  return (
    <div
      ref={containerRef}
      style={{ height }}
      className={`ace-tomorrow-night w-full border border-border-button rounded-lg overflow-hidden bg-bg-input ${className} `}
    />
  );
};

export default JsonEditor;
