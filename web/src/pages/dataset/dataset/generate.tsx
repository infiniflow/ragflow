import SvgIcon from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { t } from 'i18next';
import { lowerFirst, toLower } from 'lodash';
import { WandSparkles } from 'lucide-react';

const MenuItem: React.FC<{ name: 'KnowledgeGraph' | 'Raptor' }> = ({
  name,
}) => {
  console.log(name, 'pppp');
  return (
    <div className="flex items-start gap-2 flex-col">
      <div className="flex justify-start text-text-primary items-center gap-2">
        <SvgIcon name={`data-flow/${toLower(name)}`} width={24}></SvgIcon>
        {t(`knowledgeDetails.${lowerFirst(name)}`)}
      </div>
      <div className="text-text-secondary text-sm">
        {t(`knowledgeDetails.generate${name}`)}
      </div>
    </div>
  );
};

const Generate: React.FC = () => {
  return (
    <div className="generate">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant={'transparent'}>
            <WandSparkles className="mr-2" />
            {t('knowledgeDetails.generate')}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-[380px] p-2  ">
          <DropdownMenuItem className="border cursor-pointer p-2 rounded-md hover:border-accent-primary hover:bg-[rgba(59,160,92,0.1)]">
            <MenuItem name="KnowledgeGraph" />
          </DropdownMenuItem>
          <DropdownMenuItem
            className="border cursor-pointer p-2 rounded-md mt-3 hover:border-accent-primary hover:bg-[rgba(59,160,92,0.1)]"
            onSelect={(e) => {
              e.preventDefault();
            }}
            onClick={(e) => {
              e.stopPropagation();
            }}
          >
            <MenuItem name="Raptor" />
            {/* <div className="flex items-start gap-2 flex-col">
              <div className="flex items-center gap-2">
                <SvgIcon name={`data-flow/raptor`} width={24}></SvgIcon>
                {t('knowledgeDetails.raptor')}
              </div>
              <div>{t('knowledgeDetails.generateRaptor')}</div>
            </div> */}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
};

export default Generate;
