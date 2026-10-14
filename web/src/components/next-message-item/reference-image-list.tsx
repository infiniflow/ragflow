/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import Image, { useDocumentImageUrl } from '@/components/image';
import FileIcon from '@/components/file-icon';
import {
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
} from '@/components/ui/carousel';
import { useSetModalState } from '@/hooks/common-hooks';
import { IReferenceChunk } from '@/interfaces/database/chat';
import PdfDrawer from '@/pages/next-search/document-preview-modal';
import { buildCitationDisplayMap } from '@/utils/chat';
import { middleEllipsis } from '@/utils/common-util';
import { isPlainObject } from 'lodash';
import { RotateCw, ZoomIn, ZoomOut } from 'lucide-react';
import { useMemo } from 'react';
import { PhotoProvider, PhotoView } from 'react-photo-view';
type IProps = {
  referenceChunks?: IReferenceChunk[] | Record<string, IReferenceChunk>;
  messageContent: string;
};

type ImageItem = {
  id: string;
  documentId: string;
  documentName: string;
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

/**
 * ImagePhotoView renders an image with PhotoView preview wrapper.
 * Only wraps with PhotoView when the image URL is ready to prevent
 * PhotoView from registering an empty src which causes blank preview.
 *
 * @param id - The image identifier used to fetch the image
 * @param documentId - The document identifier for the image
 * @param index - The display index of the image in the list
 */
function ImagePhotoView({ id, documentId, documentName, index }: ImageItem) {
  const src = useDocumentImageUrl(id, documentId);
  const { visible, showModal, hideModal } = useSetModalState();
  const previewButton = (
    <button
      type="button"
      title={documentName}
      onClick={(e) => {
        // Prevent triggering the image fullscreen preview
        e.stopPropagation();
        showModal();
      }}
      className="absolute bottom-2 left-2 flex max-w-[calc(100%-2.5rem)] items-end gap-1 rounded-md text-[11px] leading-tight "
    >
      <span className="[&>img]:h-4 [&>img]:w-4 [&>svg]:h-4 [&>svg]:w-4">
        <FileIcon id={documentId} name={documentName} width={14}></FileIcon>
      </span>
      <span className="truncate">{middleEllipsis(documentName, 10, 6)}</span>
    </button>
  );
  const imageElement = (
    <Image
      id={id}
      documentId={documentId}
      className="h-40 w-full"
      label={`[${index + 1}]`}
      previewButton={previewButton}
    />
  );

  const wrapped = (
    <div className="relative">
      {imageElement}
      {visible && (
        <PdfDrawer
          visible={visible}
          hideModal={hideModal}
          documentId={documentId}
          chunk={{ document_name: documentName }}
        ></PdfDrawer>
      )}
    </div>
  );

  // When src is not yet available, render image without PhotoView
  // to avoid PhotoView caching an empty src that would show blank preview
  if (!src) {
    return wrapped;
  }

  return <PhotoView src={src}>{wrapped}</PhotoView>;
}

function ImageCarousel({ images }: { images: ImageItem[] }) {
  const buttonVisibilityClass = getButtonVisibilityClass(images.length);

  return (
    <PhotoProvider
      // className="[&_.PhotoView-Slider__toolbarIcon]:hidden"
      toolbarRender={({ rotate, onRotate, scale, onScale }) => {
        return (
          <>
            <RotateCw
              className="mr-4 cursor-pointer text-text-disabled hover:text-text-primary"
              onClick={() => onRotate(rotate + 90)}
            />
            <ZoomIn
              className="mr-4 cursor-pointer text-text-disabled hover:text-text-primary"
              onClick={() => onScale(scale + 1)}
            />
            <ZoomOut
              className="cursor-pointer text-text-disabled hover:text-text-primary"
              onClick={() => onScale(scale - 1)}
            />
            {/* <X className="cursor-pointer text-text-disabled hover:text-text-primary" /> */}
          </>
        );
      }}
    >
      <Carousel
        className="w-full"
        opts={{
          align: 'start',
        }}
      >
        <CarouselContent>
          {images.map(({ id, documentId, documentName, index }) => (
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
              <ImagePhotoView
                id={id}
                documentId={documentId}
                documentName={documentName}
                index={index}
              ></ImagePhotoView>
            </CarouselItem>
          ))}
        </CarouselContent>
        <CarouselPrevious className={buttonVisibilityClass} />
        <CarouselNext className={buttonVisibilityClass} />
      </Carousel>
    </PhotoProvider>
  );
}

export function ReferenceImageList({
  referenceChunks,
  messageContent,
}: IProps) {
  const images = useMemo(() => {
    // Same positional mapping as the citation chips in next-markdown-content:
    // each unique marker is a 0-based position by first occurrence and the
    // chunk is read by insertion order. This works for both plain chat and
    // agent references (whose markers are [ID:<chunkId>] and chunks are arrays).
    const displayIndexes = [
      ...buildCitationDisplayMap(messageContent).values(),
    ];
    const orderedChunks = Array.isArray(referenceChunks)
      ? referenceChunks
      : isPlainObject(referenceChunks)
        ? Object.values(referenceChunks ?? {})
        : [];

    return displayIndexes
      .map((position) => {
        const chunk = orderedChunks[position];
        if (!chunk?.image_id || !chunk?.document_id) {
          return null;
        }
        return {
          id: chunk.image_id,
          documentId: chunk.document_id,
          documentName: chunk.document_name,
          index: position,
        } as ImageItem;
      })
      .filter((item): item is ImageItem => item !== null);
  }, [messageContent, referenceChunks]);

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
