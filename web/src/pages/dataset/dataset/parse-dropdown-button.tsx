import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { IDocumentInfo } from '@/interfaces/database/document';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { UseChangeDocumentParserShowType } from './use-change-document-parser';

export function ParseDropdownButton({
  record,
  showChangeParserModal,
}: {
  record: IDocumentInfo;
} & UseChangeDocumentParserShowType) {
  const { t } = useTranslation();
  const { pipeline_id, pipeline_name, chunk_method } = record;

  const handleShowChangeParserModal = useCallback(() => {
    showChangeParserModal(record);
  }, [record, showChangeParserModal]);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="static" size="auto" className="capitalize">
                {pipeline_id
                  ? pipeline_name || pipeline_id
                  : chunk_method === 'naive'
                    ? 'general'
                    : chunk_method}
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              <p className="capitalize">
                {pipeline_id
                  ? pipeline_name || pipeline_id
                  : chunk_method === 'naive'
                    ? 'general'
                    : chunk_method}
              </p>
            </TooltipContent>
          </Tooltip>
        </div>
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuItem onClick={handleShowChangeParserModal}>
          {t('knowledgeDetails.dataPipeline')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
