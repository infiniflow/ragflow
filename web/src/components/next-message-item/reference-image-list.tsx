import Image from '@/components/image';
import {
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
} from '@/components/ui/carousel';
import { IReferenceChunk } from '@/interfaces/database/chat';
import { useResponsive } from 'ahooks';
import { useMemo } from 'react';
import { extractNumbersFromMessageContent } from './utils';

type IProps = {
  referenceChunks: IReferenceChunk[];
  messageContent: string;
};

function ImageCarousel({
  imageIds,
  hideButtons,
}: {
  hideButtons?: boolean;
  imageIds: string[];
}) {
  return (
    <Carousel
      className="w-full"
      opts={{
        align: 'start',
      }}
    >
      <CarouselContent>
        {imageIds.map((imageId, index) => (
          <CarouselItem key={index} className="md:basis-1/2 2xl:basis-1/6">
            <Image
              id={imageId}
              className="h-40 w-full"
              label={`Fig. ${(index + 1).toString()}`}
            />
          </CarouselItem>
        ))}
      </CarouselContent>
      {!hideButtons && (
        <>
          <CarouselPrevious />
          <CarouselNext />
        </>
      )}
    </Carousel>
  );
}

export function ReferenceImageList({
  referenceChunks,
  messageContent,
}: IProps) {
  const imageIds = useMemo(() => {
    return referenceChunks
      .filter((_, idx) =>
        extractNumbersFromMessageContent(messageContent).includes(idx),
      )
      .map((chunk) => chunk.image_id);
  }, [messageContent, referenceChunks]);
  const imageCount = imageIds.length;

  const responsive = useResponsive();

  const { isMd, is2xl } = useMemo(() => {
    return {
      isMd: responsive.md,
      is2xl: responsive['2xl'],
    };
  }, [responsive]);

  // If there are few images, hide the previous/next buttons.
  const hideButtons = is2xl ? imageCount <= 6 : isMd ? imageCount <= 2 : false;

  if (imageCount === 0) {
    return <></>;
  }

  return <ImageCarousel imageIds={imageIds} hideButtons={hideButtons} />;
}
