import { Input } from '@/components/ui/input';
import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { PenLine } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { BeginId, Operator } from '../constant';
import { useHandleNodeNameChange } from '../hooks/use-change-node-name';
import { useIsMcp } from '../hooks/use-is-mcp';

type TitleInputProps = {
  node?: RAGFlowNodeType;
};

export function TitleInput({ node }: TitleInputProps) {
  const { t } = useTranslation();
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

  const handleBlur = useCallback(() => {
    handleNameBlur();
    setIsEditingMode(false);
  }, [handleNameBlur]);

  if (isMcp) {
    return <div className="flex-1 text-base">MCP Config</div>;
  }

  return (
    <div className="flex items-center gap-1 flex-1">
      {node?.id === BeginId ? (
        <span>{t(BeginId)}</span>
      ) : isEditingMode ? (
        <Input
          value={name}
          onBlur={handleBlur}
          onChange={handleNameChange}
        ></Input>
      ) : (
        <div className="flex items-center gap-2.5 text-base">
          {name}
          <PenLine
            onClick={switchIsEditingMode}
            className="size-3.5 text-text-secondary cursor-pointer"
          />
        </div>
      )}
    </div>
  );
}
