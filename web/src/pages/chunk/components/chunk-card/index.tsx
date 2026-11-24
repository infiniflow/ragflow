import Image from '@/components/image';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Switch } from '@/components/ui/switch';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useDatasetEditButtonDisabled } from '@/ee/hooks/logic-hooks/use-permission';
import { ChunkDetailEntry } from '@/ee/pages/chunk/components/chunk-card/chunk-detail-entry';
import { ChunkVersionEntry } from '@/ee/pages/chunk/parsed-result/add-knowledge/components/knowledge-chunk/components/chunk-creating-modal/chunk-version-entry';
import type { ChunkDocType, IChunk } from '@/interfaces/database/dataset';
import { cn } from '@/lib/utils';
import { sanitizeHtmlWithImagesAsText } from '@/utils/dom-util';
import { CheckedState } from '@radix-ui/react-checkbox';
import classNames from 'classnames';
import { PenLine, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ChunkTextMode } from '../../constant';
import styles from './index.module.less';

interface IProps {
  item: IChunk;
  checked: boolean;
  switchChunk: (available?: number, chunkIds?: string[]) => Promise<void>;
  editChunk: (chunkId: string) => void;
  removeChunk: (chunkId: string) => void;
  handleCheckboxClick: (chunkId: string, checked: boolean) => void;
  selected: boolean;
  clickChunkCard: (chunkId: string) => void;
  textMode: ChunkTextMode;
  t?: string | number; // Cache-busting key for images
}

const ChunkCard = ({
  item,
  checked,
  handleCheckboxClick,
  editChunk,
  removeChunk,
  switchChunk,
  selected,
  clickChunkCard,
  textMode,
  t: imageCacheKey,
}: IProps) => {
  const { t } = useTranslation();
  const datasetEditButtonDisabled = useDatasetEditButtonDisabled();

  const available = Number(item.available_int);
  const [enabled, setEnabled] = useState(false);
  const [switching, setSwitching] = useState(false);

  const onChange = async (checked: boolean) => {
    setEnabled(checked);
    setSwitching(true);
    try {
      await switchChunk(available === 0 ? 1 : 0, [item.chunk_id]);
    } finally {
      setSwitching(false);
    }
  };

  const handleCheck = (e: CheckedState) => {
    handleCheckboxClick(item.chunk_id, e === 'indeterminate' ? false : e);
  };

  const handleEditClick = () => {
    editChunk(item.chunk_id);
  };

  const handleDeleteClick = () => {
    removeChunk(item.chunk_id);
  };

  const handleContentClick = () => {
    clickChunkCard(item.chunk_id);
  };

  useEffect(() => {
    setEnabled(available === 1);
  }, [available]);

  const chunkType =
    ((item.doc_type_kwd &&
      String(item.doc_type_kwd)?.toLowerCase()) as ChunkDocType) || 'text';

  return (
    <Card
      as="article"
      className={classNames(
        'group relative flex-none p-3 pt-6 shadow-none transition-colors hover:border-border-default',
        selected && 'bg-text-primary/15',
      )}
    >
      {/* Corner row: the chunk type, then the entry into its details - which the
          enterprise build also uses to name the chunk by id. The version the text
          is at is one of those details, so it is not printed on the card. The
          type keeps its own box and spacing; the entry takes what the card can
          spare. */}
      <div
        className="
        absolute top-0 right-0 flex max-w-[70%] items-center gap-0.5 pr-2
        leading-none text-xs text-text-disabled
        bg-bg-card rounded-bl-2xl rounded-tr-lg
        border-l-0.5 border-b-0.5 border-border-button"
      >
        <span className="shrink-0 pl-4 py-1 mr-2">
          {t(`chunk.docType.${chunkType}`)}
        </span>
        <ChunkDetailEntry chunk={item} />
      </div>

      <div className="flex items-start justify-between gap-2.5">
        <Checkbox
          className="mt-1"
          onCheckedChange={handleCheck}
          checked={checked}
        />

        {/* Using <Tooltip> instead of <Popover> to avoid flickering when hovering over the image */}
        {item.image_id && (
          <Tooltip>
            <TooltipTrigger>
              <Image
                t={imageCacheKey}
                id={item.image_id}
                documentId={item.doc_id}
                className="mt-1 rounded !w-28 object-contain"
              />
            </TooltipTrigger>

            <TooltipContent
              className="p-0"
              align="start"
              side="left"
              sideOffset={-20}
              tabIndex={-1}
            >
              <Image
                t={imageCacheKey}
                id={item.image_id}
                documentId={item.doc_id}
                className="size-full max-w-[50vw] max-h-[50vh] object-contain"
              />
            </TooltipContent>
          </Tooltip>
        )}

        <section
          className={cn(styles.content, 'flex-1')}
          onClick={handleContentClick}
        >
          <div
            dangerouslySetInnerHTML={{
              __html: sanitizeHtmlWithImagesAsText(item.content_with_weight),
            }}
            className={classNames(
              'whitespace-pre-wrap break-words',
              textMode === ChunkTextMode.Ellipse && 'line-clamp-3',
            )}
          />
        </section>

        <div className="flex w-7 shrink-0 flex-col items-center">
          <span
            className={cn(
              'm-1.5 size-2 rounded-full group-hover:hidden group-has-[:focus-visible]:hidden',
              enabled ? 'bg-state-success' : 'bg-text-disabled',
            )}
          />

          <div className="hidden group-hover:flex group-has-[:focus-visible]:flex flex-col items-center gap-2">
            <Switch
              checked={enabled}
              onCheckedChange={onChange}
              aria-readonly
              className="!m-0 !mt-[22px]"
              disabled={datasetEditButtonDisabled || switching}
            />
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={t('common.edit')}
              title={t('common.edit')}
              onClick={handleEditClick}
              disabled={datasetEditButtonDisabled}
            >
              <PenLine className="size-4" />
            </Button>
            {/* The history sits above delete: it opens from the card so a reader
                can inspect what changed without opening an edit, and delete stays
                the last action in the column where a destructive control belongs. */}
            <ChunkVersionEntry chunkId={item.chunk_id} docId={item.doc_id} />
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={t('common.delete')}
              title={t('common.delete')}
              onClick={handleDeleteClick}
              disabled={datasetEditButtonDisabled}
            >
              <Trash2 className="size-4 text-state-error" />
            </Button>
          </div>
        </div>
      </div>
    </Card>
  );
};

export default ChunkCard;
