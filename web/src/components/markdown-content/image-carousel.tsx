import Image from '@/components/image';
import {
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
} from '@/components/ui/carousel';
import { IReference, IReferenceChunk } from '@/interfaces/database/chat';
import { getExtension } from '@/utils/document-util';
import { useCallback } from 'react';

interface ImageCarouselProps {
  group: Array<{
    id: string;
    fullMatch: string;
    start: number;
  }>;
  reference: IReference;
  fileThumbnails: Record<string, string>;
  onImageClick: (
    documentId: string,
    chunk: IReferenceChunk,
    isPdf: boolean,
    documentUrl?: string,
  ) => void;
}

interface ReferenceInfo {
  documentUrl?: string;
  fileThumbnail?: string;
  fileExtension?: string;
  imageId?: string;
  chunkItem?: IReferenceChunk;
  documentId?: string;
  document?: any;
}

const getReferenceInfo = (
  chunkIndex: number,
  reference: IReference,
  fileThumbnails: Record<string, string>,
): ReferenceInfo => {
  const chunks = reference?.chunks ?? [];
  const chunkItem = chunks[chunkIndex];
  const document = reference?.doc_aggs?.find(
    (x) => x?.doc_id === chunkItem?.document_id,
  );
  const documentId = document?.doc_id;
  const documentUrl = document?.url;
  const fileThumbnail = documentId ? fileThumbnails[documentId] : '';
  const fileExtension = documentId ? getExtension(document?.doc_name) : '';
  const imageId = chunkItem?.image_id;

  return {
    documentUrl,
    fileThumbnail,
    fileExtension,
    imageId,
    chunkItem,
    documentId,
    document,
  };
};

/**
 * Component to render image carousel for a group of consecutive image references
 */
export const ImageCarousel = ({
  group,
  reference,
  fileThumbnails,
  onImageClick,
}: ImageCarouselProps) => {
  const getChunkIndex = (match: string) => Number(match);

  const handleImageClick = useCallback(
    (
      imageId: string,
      chunkItem: IReferenceChunk,
      documentId: string,
      fileExtension: string,
      documentUrl?: string,
    ) =>
      () =>
        onImageClick(
          documentId,
          chunkItem,
          fileExtension === 'pdf',
          documentUrl,
        ),
    [onImageClick],
  );

  return (
    <Carousel
      className="w-44"
      opts={{
        align: 'start',
        skipSnaps: false,
      }}
    >
      <CarouselContent>
        {group.map((ref) => {
          const chunkIndex = getChunkIndex(ref.id);
          const { documentUrl, fileExtension, imageId, chunkItem, documentId } =
            getReferenceInfo(chunkIndex, reference, fileThumbnails);

          return (
            <CarouselItem key={ref.id}>
              <section>
                <Image
                  id={imageId!}
                  className="object-contain max-h-36"
                  onClick={
                    documentId && chunkItem
                      ? handleImageClick(
                          imageId!,
                          chunkItem,
                          documentId,
                          fileExtension!,
                          documentUrl,
                        )
                      : () => {}
                  }
                />
                <span className="text-accent-primary">{imageId}</span>
              </section>
            </CarouselItem>
          );
        })}
      </CarouselContent>
      <CarouselPrevious className="h-8 w-8" />
      <CarouselNext className="h-8 w-8" />
    </Carousel>
  );
};

export default ImageCarousel;
