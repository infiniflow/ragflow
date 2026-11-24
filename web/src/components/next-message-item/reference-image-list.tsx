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
import PdfSheet from '@/components/pdf-drawer';
import {
  Carousel,
  CarouselContent,
  CarouselItem,
  CarouselNext,
  CarouselPrevious,
} from '@/components/ui/carousel';
import { useSetModalState } from '@/hooks/common-hooks';
import { IReferenceChunk } from '@/interfaces/database/chat';
import { buildCitationDisplayMap, resolveCitationChunk } from '@/utils/chat';
import { middleEllipsis } from '@/utils/common-util';
import { RotateCw, ZoomIn, ZoomOut } from 'lucide-react';
import { useMemo, type MouseEvent } from 'react';
import { PhotoProvider, PhotoView } from 'react-photo-view';
import { Button } from '../ui/button';
type IProps = {
  referenceChunks?: IReferenceChunk[] | Record<string, IReferenceChunk>;
  messageContent: string;
};

type ImageItem = {
  id: string;
  documentId: string;
  documentName: string;
  index: number;
  chunk: IReferenceChunk;
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
function ImagePhotoView({
  id,
  documentId,
  documentName,
  index,
  chunk,
}: ImageItem) {
  const src = useDocumentImageUrl(id, documentId);
  const { visible, showModal, hideModal } = useSetModalState();
  const handlePreviewClick = (event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation();
    showModal();
  };

  const card = (
    <div className="relative">
      <div className="relative">
        <Image
          id={id}
          documentId={documentId}
          className="h-40 w-full"
          label={`[${index + 1}]`}
        />
      </div>
      <Button
        variant="secondary"
        title={documentName}
        onClick={handlePreviewClick}
        // className="mt-2 flex w-full min-w-0 items-center gap-1 text-left text-[11px] leading-tight text-text-secondary "
      >
        <span className="shrink-0 [&>img]:h-4 [&>img]:w-4 [&>svg]:h-4 [&>svg]:w-4">
          <FileIcon id={documentId} name={documentName} width={14}></FileIcon>
        </span>
        <span className="min-w-0 flex-1 truncate">
          {middleEllipsis(documentName, 10, 6)}
        </span>
      </Button>
    </div>
  );

  return (
    <div className="relative">
      {/* Keep the sheet outside PhotoView: portal events still bubble through
          the React tree, so a close click inside the sheet would open preview. */}
      {src ? <PhotoView src={src}>{card}</PhotoView> : card}
      {visible && (
        <PdfSheet
          visible={visible}
          hideModal={hideModal}
          documentId={documentId}
          chunk={chunk}
        ></PdfSheet>
      )}
    </div>
  );
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
          {images.map(({ id, documentId, documentName, index, chunk }) => (
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
                chunk={chunk}
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
    // The marker itself selects the chunk: plain-chat numeric markers index
    // the retrieved chunks array (chip renders n + 1); agent [ID:<chunkId>]
    // markers match the chunk's own id. Either way a citation lands on its
    // own chunk regardless of the payload's retrieval order.
    const keyToPosition = buildCitationDisplayMap(messageContent);

    return [...keyToPosition.entries()]
      .map(([key, position]) => {
        const chunk = resolveCitationChunk(referenceChunks, key);
        if (!chunk?.image_id || !chunk?.document_id) {
          return null;
        }
        return {
          id: chunk.image_id,
          documentId: chunk.document_id,
          documentName: chunk.document_name,
          index: typeof key === 'number' ? key : position,
          chunk,
        } as ImageItem;
      })
      .filter((item): item is ImageItem => item !== null)
      .sort((a, b) => a.index - b.index);
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
