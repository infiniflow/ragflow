import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { WandSparkles } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { GenerateType } from './constants';
import { ITraceInfo, useDatasetGenerate, useTraceGenerate } from './hook';
import MenuItem from './menu-item';

type GenerateProps = {
  disabled?: boolean;
};

function GenerateDropdownMenu(props: GenerateProps) {
  const { disabled = false } = props;
  const [open, setOpen] = useState(false);
  const { graphRunData, artifactRunData, skillRunData } = useTraceGenerate({
    open,
  });
  const { runGenerate, pauseGenerate } = useDatasetGenerate();
  const { t } = useTranslation();
  const handleOpenChange = (isOpen: boolean) => {
    setOpen(isOpen);
  };

  return (
    <DropdownMenu open={open} onOpenChange={handleOpenChange}>
      <DropdownMenuTrigger asChild disabled={disabled}>
        <div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                disabled={disabled}
                className={cn(disabled && '!cursor-not-allowed')}
                variant="transparent"
                size="icon"
                onClick={() => {
                  if (!disabled) {
                    handleOpenChange(!open);
                  }
                }}
              >
                <WandSparkles />
              </Button>
            </TooltipTrigger>

            <TooltipContent>{t('knowledgeDetails.generate')}</TooltipContent>
          </Tooltip>
        </div>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-[380px] p-5 flex flex-col gap-2 ">
        {Object.values(GenerateType)
          .filter((name) => name !== GenerateType.Raptor)
          .map((name) => {
            const data = (
              name === GenerateType.KnowledgeGraph
                ? graphRunData
                : name === GenerateType.Artifact
                  ? artifactRunData
                  : skillRunData
            ) as ITraceInfo;
            return (
              <div key={name}>
                <MenuItem
                  name={name}
                  runGenerate={runGenerate}
                  data={data}
                  pauseGenerate={pauseGenerate}
                />
              </div>
            );
          })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export default GenerateDropdownMenu;
