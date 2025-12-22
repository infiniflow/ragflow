import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { PenLine } from 'lucide-react';
import { useCallback, useLayoutEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { BeginId, Operator } from '../constant';
import { useHandleNodeNameChange } from '../hooks/use-change-node-name';
import { useIsMcp } from '../hooks/use-is-mcp';

type TitleInputProps = {
  node?: RAGFlowNodeType;
};

export function TitleInput({ node }: TitleInputProps) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  const { name, handleNameBlur, handleNameChange } = useHandleNodeNameChange({
    id: node?.id,
    data: node?.data,
  });

  const operatorName: Operator = node?.data.label as Operator;
  const isMcp = useIsMcp(operatorName);
  const [isEditingMode, setIsEditingMode] = useState(false);

  const switchIsEditingMode = useCallback(() => {
    setIsEditingMode((prev) => !prev);
  }, []);

  const handleBlur = useCallback(
    (e: React.FocusEvent<HTMLInputElement>) => {
      if (handleNameBlur()) {
        setIsEditingMode(false);
      } else {
        // Re-focus the input if name doesn't change successfully
        e.target.focus();
        e.target.select();
      }
    },
    [handleNameBlur],
  );

  useLayoutEffect(() => {
    if (isEditingMode && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [isEditingMode]);

  if (isMcp) {
    return <div className="flex-1 text-base">MCP Config</div>;
  }

  return (
    // Give a fixed height to prevent layout shift when switching between edit and view modes
    <div className="flex items-center gap-1 flex-1 h-8 mr-2">
      {node?.id === BeginId ? (
        // Begin node is not editable
        <span>{t(`flow.${BeginId}`)}</span>
      ) : isEditingMode ? (
        <Input
          ref={inputRef}
          value={name}
          onBlur={handleBlur}
          onKeyDown={(e) => {
            // Support committing the value changes by pressing Enter
            if (e.key === 'Enter') {
              handleBlur(e as unknown as React.FocusEvent<HTMLInputElement>);
            }
          }}
          onChange={handleNameChange}
        />
      ) : (
        <div className="flex items-center gap-2.5 text-base">
          {name}

          <Button
            variant="transparent"
            size="icon"
            className="size-6 !p-0 border-0 bg-transparent"
            onClick={switchIsEditingMode}
          >
            <PenLine className="size-3.5 text-text-secondary cursor-pointer" />
          </Button>
        </div>
      )}
    </div>
  );
}
