import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Card, CardContent } from '@/components/ui/card';
import { IFlow } from '@/interfaces/database/agent';
import { cn } from '@/lib/utils';

interface CanvasCardProps {
  canvas: IFlow;
  selected?: boolean;
  onClick: () => void;
}

export function CanvasCard({ canvas, selected, onClick }: CanvasCardProps) {
  return (
    <Card
      onClick={onClick}
      className={cn(
        'cursor-pointer hover:shadow-md transition-shadow',
        selected && 'bg-bg-card',
      )}
    >
      <CardContent className="p-3 flex gap-2 items-start">
        <RAGFlowAvatar
          className="w-[32px] h-[32px] flex-shrink-0"
          avatar={canvas.avatar}
          name={canvas.title}
        />
        <div className="flex-1 min-w-0">
          <div className="text-base font-bold truncate">{canvas.title}</div>
          {canvas.description && (
            <div className="text-sm text-text-secondary truncate">
              {canvas.description}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
