import Image from '@/components/image';
import { IChunk } from '@/interfaces/database/knowledge';
import { Card, Checkbox, CheckboxProps, Flex, Popover, Switch } from 'antd';
import classNames from 'classnames';
import DOMPurify from 'dompurify';
import { useEffect, useState } from 'react';

import { useTheme } from '@/components/theme-provider';
import { ChunkTextMode } from '../../constant';
import styles from './index.less';

interface IProps {
  item: IChunk;
  checked: boolean;
  switchChunk: (available?: number, chunkIds?: string[]) => void;
  editChunk: (chunkId: string) => void;
  handleCheckboxClick: (chunkId: string, checked: boolean) => void;
  selected: boolean;
  clickChunkCard: (chunkId: string) => void;
  textMode: ChunkTextMode;
}

interface SpeakerSegment {
  speaker: string;
  text: string;
}

// Speaker color classes for different speakers
const speakerColors = [
  'text-blue-600 bg-blue-50 border-blue-200',
  'text-green-600 bg-green-50 border-green-200',
  'text-purple-600 bg-purple-50 border-purple-200',
  'text-orange-600 bg-orange-50 border-orange-200',
  'text-red-600 bg-red-50 border-red-200',
  'text-indigo-600 bg-indigo-50 border-indigo-200',
  'text-yellow-600 bg-yellow-50 border-yellow-200',
  'text-pink-600 bg-pink-50 border-pink-200',
];

function parseSpeakerText(content: string): SpeakerSegment[] {
  console.log(
    '🔍 [Speaker Parser] Input content:',
    content.substring(0, 200) + '...',
  );

  // Remove HTML tags first and get clean text
  const cleanContent = content.replace(/<[^>]*>/g, '').trim();
  console.log(
    '🧹 [Speaker Parser] Clean content (no HTML):',
    cleanContent.substring(0, 200) + '...',
  );

  // Match [SPEAKER_XX] pattern followed by text
  const speakerRegex = /\[SPEAKER_(\d+)\]\s*([^[]*?)(?=\[SPEAKER_\d+\]|$)/g;
  const rawSegments: SpeakerSegment[] = [];
  let match;

  // First, extract all raw segments
  while ((match = speakerRegex.exec(cleanContent)) !== null) {
    const speakerNumber = match[1];
    const text = match[2].trim();
    console.log(
      `🎤 [Speaker Parser] Found raw segment: SPEAKER_${speakerNumber} -> "${text.substring(0, 50)}${text.length > 50 ? '...' : ''}"`,
    );

    if (text) {
      rawSegments.push({
        speaker: `SPEAKER_${speakerNumber}`,
        text: text,
      });
    }
  }

  console.log(`📊 [Speaker Parser] Raw segments found: ${rawSegments.length}`);

  // Now concatenate consecutive segments from the same speaker
  const consolidatedSegments: SpeakerSegment[] = [];

  for (let i = 0; i < rawSegments.length; i++) {
    const currentSegment = rawSegments[i];

    // Check if this is the same speaker as the last consolidated segment
    if (
      consolidatedSegments.length > 0 &&
      consolidatedSegments[consolidatedSegments.length - 1].speaker ===
        currentSegment.speaker
    ) {
      // Same speaker - concatenate the text
      const lastSegment = consolidatedSegments[consolidatedSegments.length - 1];
      lastSegment.text += ' ' + currentSegment.text;
      console.log(
        `🔗 [Speaker Parser] Concatenated with previous ${currentSegment.speaker} segment`,
      );
    } else {
      // Different speaker - add as new segment
      consolidatedSegments.push({
        speaker: currentSegment.speaker,
        text: currentSegment.text,
      });
      console.log(
        `➕ [Speaker Parser] Added new ${currentSegment.speaker} segment`,
      );
    }
  }

  console.log(
    `🎯 [Speaker Parser] Final consolidated segments: ${consolidatedSegments.length}`,
  );
  consolidatedSegments.forEach((seg, idx) => {
    console.log(
      `   ${idx + 1}. ${seg.speaker}: "${seg.text.substring(0, 50)}${seg.text.length > 50 ? '...' : ''}"`,
    );
  });

  return consolidatedSegments;
}

function renderContent(item: IChunk, textMode: ChunkTextMode) {
  console.log('🎨 [Render Content] Starting render for chunk:', item.chunk_id);
  console.log('🎨 [Render Content] Text mode:', textMode);
  console.log(
    '🎨 [Render Content] Raw content:',
    item.content_with_weight.substring(0, 150) + '...',
  );

  const segments = parseSpeakerText(item.content_with_weight);

  // If no speaker patterns found, render original content
  if (segments.length === 0) {
    console.log(
      '❌ [Render Content] No speaker segments found - rendering original content',
    );
    return (
      <div
        dangerouslySetInnerHTML={{
          __html: DOMPurify.sanitize(item.content_with_weight),
        }}
        className={classNames(styles.contentText, {
          [styles.contentEllipsis]: textMode === ChunkTextMode.Ellipse,
        })}
      />
    );
  }

  console.log(
    `✅ [Render Content] Found ${segments.length} speaker segments - rendering enhanced UI`,
  );

  // Render beautiful speaker segments
  return (
    <div
      className={classNames(styles.contentText, {
        [styles.contentEllipsis]: textMode === ChunkTextMode.Ellipse,
      })}
    >
      <div className="space-y-3">
        {segments.map((segment, index) => {
          const speakerNum = parseInt(segment.speaker.split('_')[1] || '0');
          const colorClass = speakerColors[speakerNum % speakerColors.length];

          console.log(
            `🎨 [Render Content] Rendering segment ${index + 1}: Speaker ${speakerNum + 1} with color ${colorClass}`,
          );

          return (
            <div key={index} className="space-y-2">
              {/* Speaker Badge */}
              <div
                className={`inline-flex items-center px-3 py-1 rounded-full text-xs font-medium border ${colorClass}`}
              >
                🎤 Speaker {speakerNum + 1}
              </div>
              {/* Speaker Text */}
              <div className="text-gray-800 text-sm leading-relaxed pl-4 border-l-3 border-gray-300 bg-gray-50 p-3 rounded-r-lg shadow-sm">
                {segment.text}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
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
}: IProps) => {
  // console.log('🧩 [ChunkCard] Rendering chunk card for:', item.chunk_id);
  // console.log('🧩 [ChunkCard] Content preview:', item.content_with_weight.substring(0, 100) + '...');
  // console.log('🧩 [ChunkCard] Text mode:', textMode, 'Selected:', selected, 'Checked:', checked);

  const available = Number(item.available_int);
  const [enabled, setEnabled] = useState(false);
  const { theme } = useTheme();

  const onChange = (checked: boolean) => {
    setEnabled(checked);
    switchChunk(available === 0 ? 1 : 0, [item.chunk_id]);
  };

  const handleCheck: CheckboxProps['onChange'] = (e) => {
    handleCheckboxClick(item.chunk_id, e.target.checked);
  };

  const handleContentDoubleClick = () => {
    console.log(
      '🖱️ [ChunkCard] Double-clicked chunk for editing:',
      item.chunk_id,
    );
    editChunk(item.chunk_id);
  };

  const handleContentClick = () => {
    console.log('🖱️ [ChunkCard] Clicked chunk:', item.chunk_id);
    clickChunkCard(item.chunk_id);
  };

  useEffect(() => {
    setEnabled(available === 1);
  }, [available]);

  console.log(
    '🧩 [ChunkCard] About to render content section for chunk:',
    item.chunk_id,
  );

  return (
    <Card
      className={classNames(styles.chunkCard, {
        [`${theme === 'dark' ? styles.cardSelectedDark : styles.cardSelected}`]:
          selected,
      })}
    >
      <Flex gap={'middle'} justify={'space-between'}>
        <Checkbox onChange={handleCheck} checked={checked}></Checkbox>
        {item.image_id && (
          <Popover
            placement="right"
            content={
              <Image id={item.image_id} className={styles.imagePreview}></Image>
            }
          >
            <Image id={item.image_id} className={styles.image}></Image>
          </Popover>
        )}

        <section
          onDoubleClick={handleContentDoubleClick}
          onClick={handleContentClick}
          className={styles.content}
        >
          {renderContent(item, textMode)}
        </section>

        <div>
          <Switch checked={enabled} onChange={onChange} />
        </div>
      </Flex>
    </Card>
  );
};

export default ChunkCard;
