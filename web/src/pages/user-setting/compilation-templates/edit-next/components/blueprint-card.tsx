import { Radio } from '@/components/ui/radio';
import { IWikiPreset } from '@/interfaces/database/compilation-template';
import { MouseEvent, useCallback } from 'react';

type BlueprintCardProps = {
  preset: IWikiPreset;
  selected: boolean;
  onToggle: (preset: IWikiPreset) => void;
  onPreview: (preset: IWikiPreset) => void;
};

export function BlueprintCard({
  preset,
  selected,
  onToggle,
  onPreview,
}: BlueprintCardProps) {
  const handleClick = useCallback(() => {
    onPreview(preset);
  }, [onPreview, preset]);

  const handleRadioClick = useCallback(
    (event: MouseEvent<HTMLInputElement>) => {
      event.stopPropagation();
      onToggle(preset);
    },
    [onToggle, preset],
  );

  const handleRadioWrapperClick = useCallback(
    (event: MouseEvent<HTMLDivElement>) => {
      event.stopPropagation();
    },
    [],
  );

  return (
    <div
      className="relative cursor-pointer rounded-lg border border-border-button bg-bg-card p-4 transition-colors hover:border-border-default"
      onClick={handleClick}
    >
      <div
        className="absolute right-3 top-3"
        onClick={handleRadioWrapperClick}
      >
        <Radio
          value={preset.id}
          checked={selected}
          onClick={handleRadioClick}
        />
      </div>

      <div className="pr-6 text-sm font-medium text-text-primary break-words">
        {preset.topic}
      </div>
    </div>
  );
}
