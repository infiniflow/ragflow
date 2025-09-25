import { transformFile2Base64 } from '@/utils/file-util';
import { Pencil, Upload, XIcon } from 'lucide-react';
import {
  ChangeEventHandler,
  forwardRef,
  useCallback,
  useEffect,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Avatar, AvatarFallback, AvatarImage } from './ui/avatar';
import { Button } from './ui/button';
import { Input } from './ui/input';

type AvatarUploadProps = { value?: string; onChange?: (value: string) => void };

export const AvatarUpload = forwardRef<HTMLInputElement, AvatarUploadProps>(
  function AvatarUpload({ value, onChange }, ref) {
    const { t } = useTranslation();
    const [avatarBase64Str, setAvatarBase64Str] = useState(''); // Avatar Image base64

    const handleChange: ChangeEventHandler<HTMLInputElement> = useCallback(
      async (ev) => {
        const file = ev.target?.files?.[0];
        if (/\.(jpg|jpeg|png|webp|bmp)$/i.test(file?.name ?? '')) {
          const str = await transformFile2Base64(file!);
          setAvatarBase64Str(str);
          onChange?.(str);
        }
        ev.target.value = '';
      },
      [onChange],
    );

    const handleRemove = useCallback(() => {
      setAvatarBase64Str('');
      onChange?.('');
    }, [onChange]);

    useEffect(() => {
      if (value) {
        setAvatarBase64Str(value);
      }
    }, [value]);

    return (
      <div className="flex justify-start items-end space-x-2">
        <div className="relative group">
          {!avatarBase64Str ? (
            <div className="w-[64px] h-[64px] grid place-content-center border border-dashed	rounded-md">
              <div className="flex flex-col items-center">
                <Upload />
                <p>{t('common.upload')}</p>
              </div>
            </div>
          ) : (
            <div className="w-[64px] h-[64px] relative grid place-content-center">
              <Avatar className="w-[64px] h-[64px] rounded-md">
                <AvatarImage className=" block" src={avatarBase64Str} alt="" />
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
        <div className="margin-1 text-muted-foreground">
          {t('knowledgeConfiguration.photoTip')}
        </div>
      </div>
    );
  },
);
