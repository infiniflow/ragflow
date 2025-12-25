import Image from '@/components/image';
import {
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
} from '@/components/ui/carousel';
import { IReferenceChunk } from '@/interfaces/database/chat';
import { isPlainObject } from 'lodash';
import { useMemo } from 'react';
import { extractNumbersFromMessageContent } from './utils';

type IProps = {
  referenceChunks?: IReferenceChunk[] | Record<string, IReferenceChunk>;
  messageContent: string;
};

type ImageItem = {
  id: string;
  index: number;
};

const getButtonVisibilityClass = (imageCount: number) => {
  const map: Record<number, string> = {
    1: 'hidden',
    2: '@sm:hidden',
    3: '@md:hidden',
    4: '@lg:hidden',
    5: '@lg:hidden',
  };
  return map[imageCount] || (imageCount >= 6 ? '@2xl:hidden' : '');
};

function ImageCarousel({ images }: { images: ImageItem[] }) {
  const buttonVisibilityClass = getButtonVisibilityClass(images.length);

  return (
    <Carousel
      className="w-full"
      opts={{
        align: 'start',
      }}
    >
      <CarouselContent>
        {images.map(({ id, index }) => (
          <CarouselItem
            key={index}
            className="
            basis-full
            @sm:basis-1/2
            @md:basis-1/3
            @lg:basis-1/4
            @2xl:basis-1/6
            "
          >
            <Image
              id={id}
              className="h-40 w-full"
              label={`Fig. ${(index + 1).toString()}`}
            />
          </CarouselItem>
        ))}
      </CarouselContent>
      <CarouselPrevious className={buttonVisibilityClass} />
      <CarouselNext className={buttonVisibilityClass} />
    </Carousel>
  );
}

export function ReferenceImageList({
  referenceChunks,
  messageContent,
}: IProps) {
  const allChunkIndexes = extractNumbersFromMessageContent(messageContent);
  const images = useMemo(() => {
    if (Array.isArray(referenceChunks)) {
      return referenceChunks
        .map((chunk, idx) => ({ id: chunk.image_id, index: idx }))
        .filter((item, idx) => allChunkIndexes.includes(idx) && item.id);
    }

    if (isPlainObject(referenceChunks)) {
      return Object.entries(referenceChunks || {}).reduce<ImageItem[]>(
        (pre, [idx, chunk]) => {
          if (allChunkIndexes.includes(Number(idx)) && chunk.image_id) {
            return pre.concat({ id: chunk.image_id, index: Number(idx) });
          }
          return pre;
        },
        [],
      );
    }

    return [];
  }, [allChunkIndexes, referenceChunks]);

  const imageCount = images?.length || 0;

  if (imageCount === 0) {
    return <></>;
  }

  return (
    <section className="@container w-full">
      <ImageCarousel images={images} />
    </section>
  );
}
