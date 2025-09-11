import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import { useState } from 'react';

interface FormatPreserveEditorProps {
  initialValue: string;
  onSave: (value: string) => void;
  className?: string;
}
const FormatPreserveEditor = ({
  initialValue,
  onSave,
  className,
}: FormatPreserveEditorProps) => {
  const [content, setContent] = useState(initialValue);
  const [isEditing, setIsEditing] = useState(false);

  const handleEdit = () => setIsEditing(true);

  const handleSave = () => {
    onSave(content);
    setIsEditing(false);
  };

  return (
    <div className="editor-container">
      {isEditing ? (
        <Textarea
          className={cn(
            'w-full h-full bg-transparent text-text-secondary',
            className,
          )}
          value={content}
          onChange={(e) => setContent(e.target.value)}
          onBlur={handleSave}
          autoSize={{ maxRows: 100 }}
          autoFocus
        />
      ) : (
        <pre className="text-text-secondary" onClick={handleEdit}>
          {content}
        </pre>
      )}
    </div>
  );
};

export default FormatPreserveEditor;
