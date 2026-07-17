import { useCallback, useEffect, useRef, useState } from 'react';

import LexicalEditor from '@/lib/editor/lexical-editor';
import RawMarkdownEditor from '@/lib/editor/raw-markdown-editor';

interface MarkdownEditorProps {
  content: string;
  onChange?: (content: string) => void;
  readOnly?: boolean;
  onWikiLinkClick?: (pageType: 'concept' | 'entity', slug: string) => void;
}

export default function MarkdownEditor({
  content,
  onChange,
  readOnly = false,
  onWikiLinkClick,
}: MarkdownEditorProps) {
  const [showSource, setShowSource] = useState(false);
  const [rawContent, setRawContent] = useState(content);
  const contentRef = useRef(content);

  useEffect(() => {
    contentRef.current = content;
    if (showSource) setRawContent(content);
  }, [showSource, content]);

  const handleWysiwygChange = useCallback(
    (md: string) => {
      if (readOnly) return;
      contentRef.current = md;
      onChange?.(md);
    },
    [onChange, readOnly],
  );

  const handleRawChange = useCallback(
    (value: string) => {
      if (readOnly) return;
      setRawContent(value);
      contentRef.current = value;
      onChange?.(value);
    },
    [onChange, readOnly],
  );

  const toggleSource = useCallback(() => {
    if (!showSource) setRawContent(contentRef.current);
    setShowSource((prev) => !prev);
  }, [showSource]);

  return (
    <div className="flex flex-1 flex-col min-h-0">
      <div className="nim-editor-container">
        <div
          className="flex flex-1 flex-col min-h-0"
          style={{ display: showSource ? 'none' : 'flex' }}
        >
          <LexicalEditor
            content={content}
            onChange={handleWysiwygChange}
            readOnly={readOnly}
            placeholder={readOnly ? '' : 'Start writing...'}
            onToggleSource={toggleSource}
            showSource={showSource}
            onWikiLinkClick={onWikiLinkClick}
          />
        </div>
        <div
          className="flex flex-1 flex-col min-h-0"
          style={{ display: showSource ? 'flex' : 'none' }}
        >
          {readOnly ? (
            <div className="flex-1 overflow-auto whitespace-pre-wrap break-words px-5 py-4 text-[13px] leading-relaxed font-mono bg-bg-card text-text-primary">
              <div className="flex items-center gap-1.5 px-3 py-1 border-b border-border bg-bg-base shrink-0 -mx-5 -mt-4 mb-3">
                <button
                  type="button"
                  onClick={toggleSource}
                  className="px-2.5 py-1 text-xs font-medium border border-accent-primary rounded bg-accent-primary-10 text-accent-primary hover:bg-accent-primary-20 transition-colors"
                >
                  WYSIWYG
                </button>
              </div>
              <pre className="m-0 font-inherit text-inherit leading-inherit whitespace-pre-wrap">
                {rawContent}
              </pre>
            </div>
          ) : (
            <RawMarkdownEditor
              content={rawContent}
              onChange={handleRawChange}
              readOnly={readOnly}
              onToggleSource={toggleSource}
            />
          )}
        </div>
      </div>
    </div>
  );
}
