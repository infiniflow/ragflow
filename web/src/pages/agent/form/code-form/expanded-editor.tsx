import { Button } from '@/components/ui/button';
import Editor from '@monaco-editor/react';
import { Minimize2 } from 'lucide-react';
import { CodeEditorOptions } from './monaco-config';

interface ExpandedEditorProps {
  visible: boolean;
  onClose: () => void;
  theme: string;
  language: string;
  value: string;
  onChange: (value: string) => void;
}

export function ExpandedEditor({
  visible,
  onClose,
  theme,
  language,
  value,
  onChange,
}: ExpandedEditorProps) {
  if (!visible) return null;

  return (
    <div className="absolute inset-0 z-10 flex flex-col bg-bg-base">
      <div className="flex items-center justify-between border-b px-5 py-3">
        <span className="font-medium">Code</span>
        <Button variant="ghost" onClick={onClose}>
          <Minimize2 className="size-4" />
        </Button>
      </div>
      <div className="min-h-0 flex-1 p-4">
        <Editor
          height="100%"
          theme={theme}
          language={language}
          options={CodeEditorOptions}
          value={value}
          onChange={(val) => {
            onChange(val ?? '');
          }}
        />
      </div>
    </div>
  );
}
