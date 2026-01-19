import { LlmIcon } from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import { APIMapUrl } from '@/constants/llm';
import { t } from 'i18next';
import { ArrowUpRight, Plus } from 'lucide-react';

export const LLMHeader = ({ name }: { name: string }) => {
  return (
    <div className="flex items-center space-x-3 mb-3">
      <LlmIcon name={name} imgClass="h-8 w-8 text-text-primary" />
      <div className="flex flex-1 gap-1 items-center">
        <div className="font-normal text-base truncate">{name}</div>
        {!!APIMapUrl[name as keyof typeof APIMapUrl] && (
          <Button
            variant={'ghost'}
            className=" bg-transparent w-4 h-5"
            onClick={(e) => {
              e.stopPropagation();
              window.open(APIMapUrl[name as keyof typeof APIMapUrl]);
            }}
            // target="_blank"
            rel="noopener noreferrer"
          >
            <ArrowUpRight size={16} />
          </Button>
        )}
      </div>
      <Button className=" px-2 items-center gap-0 text-xs h-6  rounded-md transition-colors hidden group-hover:flex">
        <Plus size={12} />
        {t('addTheModel')}
      </Button>
    </div>
  );
};
