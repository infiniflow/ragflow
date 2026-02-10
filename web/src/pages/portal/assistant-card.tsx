import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { MessageCircle } from 'lucide-react';
import { PublicDialog } from './hooks';

interface AssistantCardProps {
  dialog: PublicDialog;
  onClick: (dialog: PublicDialog) => void;
}

export function AssistantCard({ dialog, onClick }: AssistantCardProps) {
  return (
    <Card className="hover:shadow-lg transition-shadow cursor-pointer group">
      <CardHeader className="pb-3">
        <div className="flex items-start gap-3">
          <RAGFlowAvatar
            avatar={dialog.icon}
            name={dialog.name}
            className="size-12"
          />
          <div className="flex-1 min-w-0">
            <CardTitle className="text-lg line-clamp-1">
              {dialog.name}
            </CardTitle>
          </div>
        </div>
      </CardHeader>
      <CardContent className="pb-3">
        <CardDescription className="line-clamp-2 min-h-[2.5rem]">
          {dialog.description || dialog.prologue || '暂无描述'}
        </CardDescription>
      </CardContent>
      <CardFooter>
        <Button
          className="w-full group-hover:bg-primary group-hover:text-primary-foreground"
          variant="outline"
          onClick={() => onClick(dialog)}
        >
          <MessageCircle className="mr-2 size-4" />
          开始对话
        </Button>
      </CardFooter>
    </Card>
  );
}
