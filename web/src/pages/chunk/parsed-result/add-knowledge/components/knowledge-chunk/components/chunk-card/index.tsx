import Image from '@/components/image';
import { useTheme } from '@/components/theme-provider';
import { Card } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Switch } from '@/components/ui/switch';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import type { ChunkDocType, IChunk } from '@/interfaces/database/knowledge';
import { cn } from '@/lib/utils';
import { CheckedState } from '@radix-ui/react-checkbox';
import classNames from 'classnames';
import DOMPurify from 'dompurify';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ChunkTextMode } from '../../constant';
import styles from './index.module.less';

interface IProps {
  item: IChunk;
  checked: boolean;
  switchChunk: (available?: number, chunkIds?: string[]) => void;
  editChunk: (chunkId: string) => void;
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
  switchChunk,
  selected,
  clickChunkCard,
  textMode,
  t: imageCacheKey,
}: IProps) => {
  const { t } = useTranslation();
  const available = Number(item.available_int);
  const [enabled, setEnabled] = useState(false);
  const { theme } = useTheme();

  const onChange = (checked: boolean) => {
    setEnabled(checked);
    switchChunk(available === 0 ? 1 : 0, [item.chunk_id]);
  };

  const handleCheck = (e: CheckedState) => {
    handleCheckboxClick(item.chunk_id, e === 'indeterminate' ? false : e);
  };

  const handleContentDoubleClick = () => {
    editChunk(item.chunk_id);
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
        'relative flex-none p-3 pt-6 shadow-none transition-colors',
        selected && 'bg-text-primary/15',
      )}
    >
      <span
        className="
        absolute top-0 right-0 px-4 py-1
        leading-none text-xs text-text-disabled
        bg-bg-card rounded-bl-2xl rounded-tr-lg
        border-l-0.5 border-b-0.5 border-border-button"
      >
        {t(`chunk.docType.${chunkType}`)}
      </span>

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
                className="size-full max-w-[50vw] max-h-[50vh] object-contain"
              />
            </TooltipContent>
          </Tooltip>
        )}

        <section
          className={cn(styles.content, 'flex-1')}
          onDoubleClick={handleContentDoubleClick}
          onClick={handleContentClick}
        >
          <div
            dangerouslySetInnerHTML={{
              __html: DOMPurify.sanitize(item.content_with_weight).trim(),
            }}
            className={classNames(
              // Keep whitespaces?
              'text-wrap break-words whitespace-pre',
              textMode === ChunkTextMode.Ellipse && 'line-clamp-3',
            )}
          />
        </section>

        <div>
          <Switch
            checked={enabled}
            onCheckedChange={onChange}
            aria-readonly
            className="!m-0"
          />
        </div>
      </div>
    </Card>
  );
};

export default ChunkCard;
