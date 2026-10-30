import {
  CanvasBackgroundPresets,
  CanvasBackgroundSetting,
  readCanvasBackgroundImage,
} from '@/components/canvas/canvas-background';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import { Paintbrush } from 'lucide-react';
import { ChangeEvent, useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

export function CanvasBackgroundControl({
  setting,
  onChange,
}: {
  setting: CanvasBackgroundSetting;
  onChange: (next: CanvasBackgroundSetting) => void;
}) {
  const { t } = useTranslation();
  const [imageError, setImageError] = useState(false);

  const chooseColor = useCallback(
    (color: string) => {
      setImageError(false);
      onChange({ mode: 'color', color });
    },
    [onChange],
  );

  const chooseImage = useCallback(
    async (event: ChangeEvent<HTMLInputElement>) => {
      const file = event.target.files?.[0];
      event.target.value = '';
      if (!file) return;
      setImageError(false);
      let image: string;
      try {
        image = await readCanvasBackgroundImage(file, setting.color);
      } catch {
        setImageError(true);
        return;
      }
      await onChange({ mode: 'image', image, color: setting.color });
    },
    [onChange, setting.color],
  );

  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="react-flow__controls-button !m-0 !box-border !flex !size-8 !items-center !justify-center !border-y-0 !border-l-0 !border-r !border-solid !border-border-button !bg-transparent !p-0 text-text-primary last:!border-r-0 hover:!bg-bg-base-hover"
          aria-label={t('flow.canvasBackground')}
        >
          <Paintbrush className="!h-3 !w-3 !fill-none" />
        </button>
      </PopoverTrigger>
      <PopoverContent side="top" align="end" className="w-64 space-y-3 p-3">
        <p className="text-sm font-medium">{t('flow.canvasBackground')}</p>
        <div className="space-y-2">
          <p className="text-xs text-text-secondary">
            {t('flow.canvasBackgroundColor')}
          </p>
          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              className={cn(
                'h-6 rounded-sm border border-border-button px-2 text-xs',
                setting.mode === 'default' && 'border-accent-primary',
              )}
              onClick={() => onChange({ mode: 'default' })}
            >
              {t('flow.canvasBackgroundDefault')}
            </button>
            {CanvasBackgroundPresets.map((color) => (
              <button
                key={color}
                type="button"
                aria-label={color}
                className={cn(
                  'size-6 rounded-sm border border-border-button',
                  setting.mode === 'color' &&
                    setting.color === color &&
                    'ring-2 ring-accent-primary',
                )}
                style={{ backgroundColor: color }}
                onClick={() => chooseColor(color)}
              />
            ))}
            <input
              type="color"
              aria-label={t('flow.canvasBackgroundColor')}
              className="size-6 cursor-pointer border-0 bg-transparent p-0"
              value={
                setting.mode === 'color' && setting.color?.startsWith('#')
                  ? setting.color
                  : '#0b0f14'
              }
              onChange={(event) => chooseColor(event.target.value)}
            />
          </div>
        </div>
        <div className="space-y-2">
          <p className="text-xs text-text-secondary">
            {t('flow.canvasBackgroundImage')}
          </p>
          <input
            type="file"
            accept="image/*"
            className="block w-full text-xs text-text-primary file:mr-2 file:cursor-pointer file:rounded-sm file:border-0 file:bg-bg-card file:px-2 file:py-1"
            onChange={chooseImage}
          />
          {imageError ? (
            <p className="text-xs text-state-error">
              {t('flow.canvasBackgroundImageError')}
            </p>
          ) : null}
          {setting.mode !== 'default' ? (
            <button
              type="button"
              className="text-xs text-text-secondary underline"
              onClick={() => onChange({ mode: 'default' })}
            >
              {t('flow.canvasBackgroundClear')}
            </button>
          ) : null}
        </div>
      </PopoverContent>
    </Popover>
  );
}
