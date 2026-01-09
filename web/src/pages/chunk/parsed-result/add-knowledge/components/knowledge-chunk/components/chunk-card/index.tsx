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
      className={classNames('relative flex-none', styles.chunkCard, {
        [`${theme === 'dark' ? styles.cardSelectedDark : styles.cardSelected}`]:
          selected,
      })}
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

      <div className="flex items-start justify-between gap-2">
        <Checkbox onCheckedChange={handleCheck} checked={checked}></Checkbox>

        {/* Using <Tooltip> instead of <Popover> to avoid flickering when hovering over the image */}
        {item.image_id && (
          <Tooltip>
            <TooltipTrigger>
              <Image
                t={imageCacheKey}
                id={item.image_id}
                className={styles.image}
              />
            </TooltipTrigger>
            <TooltipContent
              className="p-0"
              align={'start'}
              side={'left'}
              sideOffset={-20}
              tabIndex={-1}
            >
              <Image
                t={imageCacheKey}
                id={item.image_id}
                className={styles.imagePreview}
              />
            </TooltipContent>
          </Tooltip>
        )}

        <section
          onDoubleClick={handleContentDoubleClick}
          onClick={handleContentClick}
          className={cn(styles.content, 'mt-2')}
        >
          <div
            dangerouslySetInnerHTML={{
              __html: DOMPurify.sanitize(item.content_with_weight),
            }}
            className={classNames(styles.contentText, {
              [styles.contentEllipsis]: textMode === ChunkTextMode.Ellipse,
            })}
          ></div>
        </section>

        <div className="mt-2">
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
