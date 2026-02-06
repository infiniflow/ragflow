import { transformFile2Base64 } from '@/utils/file-util';
import { Pencil, Plus, XIcon } from 'lucide-react';
import {
  ChangeEventHandler,
  forwardRef,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Avatar, AvatarFallback, AvatarImage } from './ui/avatar';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Modal } from './ui/modal/modal';

type AvatarUploadProps = {
  value?: string;
  onChange?: (value: string) => void;
  tips?: string;
};

export const AvatarUpload = forwardRef<HTMLInputElement, AvatarUploadProps>(
  function AvatarUpload({ value, onChange, tips }, ref) {
    const { t } = useTranslation();
    const [avatarBase64Str, setAvatarBase64Str] = useState(''); // Avatar Image base64
    const [isCropModalOpen, setIsCropModalOpen] = useState(false);
    const [imageToCrop, setImageToCrop] = useState<string | null>(null);
    const [cropArea, setCropArea] = useState({ x: 0, y: 0, size: 200 });
    const imageRef = useRef<HTMLImageElement>(null);
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const containerRef = useRef<HTMLDivElement>(null);
    const isDraggingRef = useRef(false);
    const dragStartRef = useRef({ x: 0, y: 0 });
    const [imageScale, setImageScale] = useState(1);
    const [imageOffset, setImageOffset] = useState({ x: 0, y: 0 });

    const handleChange: ChangeEventHandler<HTMLInputElement> = useCallback(
      async (ev) => {
        const file = ev.target?.files?.[0];
        if (/\.(jpg|jpeg|png|webp|bmp)$/i.test(file?.name ?? '')) {
          const str = await transformFile2Base64(file!, 1000);
          setImageToCrop(str);
          setIsCropModalOpen(true);
        }
        ev.target.value = '';
      },
      [onChange],
    );

    const handleRemove = useCallback(() => {
      setAvatarBase64Str('');
      onChange?.('');
    }, [onChange]);

    const handleCrop = useCallback(() => {
      if (!imageRef.current || !canvasRef.current) return;

      const canvas = canvasRef.current;
      const ctx = canvas.getContext('2d');
      const image = imageRef.current;

      if (!ctx) return;

      // Set canvas size to 64x64 (avatar size)
      canvas.width = 64;
      canvas.height = 64;

      // Draw cropped image on canvas
      ctx.drawImage(
        image,
        cropArea.x,
        cropArea.y,
        cropArea.size,
        cropArea.size,
        0,
        0,
        64,
        64,
      );

      // Convert to base64
      const croppedImageBase64 = canvas.toDataURL('image/png');
      setAvatarBase64Str(croppedImageBase64);
      onChange?.(croppedImageBase64);
      setIsCropModalOpen(false);
    }, [cropArea, onChange]);

    const handleCancelCrop = useCallback(() => {
      setIsCropModalOpen(false);
      setImageToCrop(null);
    }, []);

    const initCropArea = useCallback(() => {
      if (!imageRef.current || !containerRef.current) return;

      const image = imageRef.current;
      const container = containerRef.current;

      // Calculate image scale to fit container
      const scale = Math.min(
        container.clientWidth / image.width,
        container.clientHeight / image.height,
      );
      setImageScale(scale);

      // Calculate image offset to center it
      const scaledWidth = image.width * scale;
      const scaledHeight = image.height * scale;
      const offsetX = (container.clientWidth - scaledWidth) / 2;
      const offsetY = (container.clientHeight - scaledHeight) / 2;
      setImageOffset({ x: offsetX, y: offsetY });

      // Initialize crop area to center of image
      const size = Math.min(scaledWidth, scaledHeight) * 0.8; // 80% of the smaller dimension
      const x = (image.width - size / scale) / 2;
      const y = (image.height - size / scale) / 2;

      setCropArea({ x, y, size: size / scale });
    }, []);

    const handleMouseMove = useCallback(
      (e: MouseEvent) => {
        if (
          !isDraggingRef.current ||
          !imageRef.current ||
          !containerRef.current
        )
          return;

        const image = imageRef.current;
        const container = containerRef.current;
        const containerRect = container.getBoundingClientRect();

        // Calculate mouse position relative to container
        const mouseX = e.clientX - containerRect.left;
        const mouseY = e.clientY - containerRect.top;

        // Calculate mouse position relative to image
        const imageX = (mouseX - imageOffset.x) / imageScale;
        const imageY = (mouseY - imageOffset.y) / imageScale;

        // Calculate new crop area position based on mouse movement
        let newX = imageX - dragStartRef.current.x;
        let newY = imageY - dragStartRef.current.y;

        // Boundary checks
        newX = Math.max(0, Math.min(newX, image.width - cropArea.size));
        newY = Math.max(0, Math.min(newY, image.height - cropArea.size));

        setCropArea((prev) => ({
          ...prev,
          x: newX,
          y: newY,
        }));
      },
      [cropArea.size, imageScale, imageOffset],
    );

    const handleMouseUp = useCallback(() => {
      isDraggingRef.current = false;
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    }, [handleMouseMove]);

    const handleMouseDown = useCallback(
      (e: React.MouseEvent) => {
        e.preventDefault();
        e.stopPropagation();
        isDraggingRef.current = true;
        if (imageRef.current && containerRef.current) {
          const container = containerRef.current;
          const containerRect = container.getBoundingClientRect();

          // Calculate mouse position relative to container
          const mouseX = e.clientX - containerRect.left;
          const mouseY = e.clientY - containerRect.top;

          // Calculate mouse position relative to image
          const imageX = (mouseX - imageOffset.x) / imageScale;
          const imageY = (mouseY - imageOffset.y) / imageScale;

          // Store the offset between mouse position and crop area position
          dragStartRef.current = {
            x: imageX - cropArea.x,
            y: imageY - cropArea.y,
          };
        }
        document.addEventListener('mousemove', handleMouseMove);
        document.addEventListener('mouseup', handleMouseUp);
      },
      [cropArea, imageScale, imageOffset],
    );

    const handleWheel = useCallback((e: React.WheelEvent) => {
      if (!imageRef.current) return;

      e.preventDefault();
      const image = imageRef.current;
      const delta = e.deltaY > 0 ? 0.9 : 1.1; // Zoom factor

      setCropArea((prev) => {
        const newSize = Math.max(
          20,
          Math.min(prev.size * delta, Math.min(image.width, image.height)),
        );

        // Adjust position to keep crop area centered
        const centerRatioX = (prev.x + prev.size / 2) / image.width;
        const centerRatioY = (prev.y + prev.size / 2) / image.height;

        const newX = centerRatioX * image.width - newSize / 2;
        const newY = centerRatioY * image.height - newSize / 2;

        // Boundary checks
        const boundedX = Math.max(0, Math.min(newX, image.width - newSize));
        const boundedY = Math.max(0, Math.min(newY, image.height - newSize));

        return {
          x: boundedX,
          y: boundedY,
          size: newSize,
        };
      });
    }, []);

    useEffect(() => {
      if (value) {
        setAvatarBase64Str(value);
      }
    }, [value]);

    useEffect(() => {
      const container = containerRef.current;
      setTimeout(() => {
        console.log('container', container);
        // initCropArea();
        if (imageToCrop && container && isCropModalOpen) {
          container.addEventListener(
            'wheel',
            handleWheel as unknown as EventListener,
            { passive: false },
          );
          return () => {
            container.removeEventListener(
              'wheel',
              handleWheel as unknown as EventListener,
            );
          };
        }
      }, 100);
    }, [handleWheel, containerRef.current]);

    return (
      <div className="flex justify-start items-end space-x-2">
        <div className="relative group">
          {!avatarBase64Str ? (
            <div className="w-[64px] h-[64px] grid place-content-center border border-dashed bg-bg-input rounded-md">
              <div className="flex flex-col items-center">
                <Plus />
                <p>{t('common.upload')}</p>
              </div>
            </div>
          ) : (
            <div className="w-[64px] h-[64px] relative grid place-content-center">
              <Avatar className="w-[64px] h-[64px] rounded-md">
                <AvatarImage className="block" src={avatarBase64Str} alt="" />
                <AvatarFallback></AvatarFallback>
              </Avatar>
              <div className="absolute inset-0 bg-[#000]/20 group-hover:bg-[#000]/60">
                <Pencil
                  size={20}
                  className="absolute right-2 bottom-0 opacity-50 hidden group-hover:block"
                />
              </div>
              <Button
                onClick={handleRemove}
                size="icon"
                className="border-background focus-visible:border-background absolute -top-2 -right-2 size-6 rounded-full border-2 shadow-none z-10"
                aria-label="Remove image"
                type="button"
              >
                <XIcon className="size-3.5" />
              </Button>
            </div>
          )}
          <Input
            placeholder=""
            type="file"
            title=""
            accept="image/*"
            className="absolute top-0 left-0 w-full h-full opacity-0 cursor-pointer"
            onChange={handleChange}
            ref={ref}
          />
        </div>
        <div className="margin-1 text-text-secondary">
          {tips ?? t('knowledgeConfiguration.photoTip')}
        </div>

        {/* Crop Modal */}
        <Modal
          open={isCropModalOpen}
          onOpenChange={(open) => {
            setIsCropModalOpen(open);
            if (!open) {
              setImageToCrop(null);
            }
          }}
          title={t('setting.cropImage')}
          size="small"
          onCancel={handleCancelCrop}
          onOk={handleCrop}
          // footer={
          //   <div className="flex justify-end space-x-2">
          //     <Button variant="secondary" onClick={handleCancelCrop}>
          //       {t('common.cancel')}
          //     </Button>
          //     <Button onClick={handleCrop}>{t('common.confirm')}</Button>
          //   </div>
          // }
        >
          <div className="flex flex-col items-center p-4">
            {imageToCrop && (
              <div className="w-full">
                <div
                  ref={containerRef}
                  className="relative overflow-hidden border border-border rounded-md mx-auto bg-bg-card"
                  style={{
                    width: '300px',
                    height: '300px',
                    touchAction: 'none',
                  }}
                  // onWheel={handleWheel}
                >
                  <img
                    ref={imageRef}
                    src={imageToCrop}
                    alt="To crop"
                    className="absolute block"
                    style={{
                      transform: `scale(${imageScale})`,
                      transformOrigin: 'top left',
                      left: `${imageOffset.x}px`,
                      top: `${imageOffset.y}px`,
                    }}
                    onLoad={initCropArea}
                  />
                  {imageRef.current && (
                    <div
                      className="absolute border-2 border-white border-dashed cursor-move"
                      style={{
                        left: `${imageOffset.x + cropArea.x * imageScale}px`,
                        top: `${imageOffset.y + cropArea.y * imageScale}px`,
                        width: `${cropArea.size * imageScale}px`,
                        height: `${cropArea.size * imageScale}px`,
                        boxShadow: '0 0 0 9999px rgba(0, 0, 0, 0.5)',
                      }}
                      onMouseDown={handleMouseDown}
                    />
                  )}
                </div>
                <div className="flex justify-center mt-4">
                  <p className="text-sm text-text-secondary">
                    {t('setting.cropTip')}
                  </p>
                </div>
                <canvas ref={canvasRef} className="hidden" />
              </div>
            )}
          </div>
        </Modal>
      </div>
    );
  },
);
