import { LlmModelType } from '@/constants/knowledge';
import * as SelectPrimitive from '@radix-ui/react-select';
import { forwardRef, memo, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { LlmSettingFieldItems } from '../llm-setting-items/next';
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover';
import { Select, SelectTrigger, SelectValue } from '../ui/select';
import LLMLabel from './llm-label';

export interface NextInnerLLMSelectProps {
  id?: string;
  value?: string;
  onInitialValue?: (value: string, option: any) => void;
  onChange?: (value: string) => void;
  disabled?: boolean;
  filter?: string;
  showSpeech2TextModel?: boolean;
  triggerTestId?: string;
  optionTestIdPrefix?: string;
}

const NextInnerLLMSelect = forwardRef<
  React.ElementRef<typeof SelectPrimitive.Trigger>,
  NextInnerLLMSelectProps
>(
  (
    {
      value,
      disabled,
      filter,
      showSpeech2TextModel = false,
      triggerTestId,
      optionTestIdPrefix,
    },
    ref,
  ) => {
    const { t } = useTranslation();
    const [isPopoverOpen, setIsPopoverOpen] = useState(false);

    const ttsModel = useMemo(() => {
      return showSpeech2TextModel ? [LlmModelType.Speech2text] : [];
    }, [showSpeech2TextModel]);

    const modelTypes = useMemo(() => {
      if (filter === LlmModelType.Chat) {
        return [LlmModelType.Chat];
      } else if (filter === LlmModelType.Image2text) {
        return [LlmModelType.Image2text, ...ttsModel];
      } else {
        return [LlmModelType.Chat, LlmModelType.Image2text, ...ttsModel];
      }
    }, [filter, ttsModel]);

    return (
      <Select disabled={disabled} value={value}>
        <Popover open={isPopoverOpen} onOpenChange={setIsPopoverOpen}>
          <PopoverTrigger asChild>
            <SelectTrigger
              onClick={(e) => {
                e.preventDefault();
                setIsPopoverOpen(true);
              }}
              ref={ref}
              data-testid={triggerTestId}
            >
              <SelectValue placeholder={t('common.pleaseSelect')}>
                <LLMLabel value={value} />
              </SelectValue>
            </SelectTrigger>
          </PopoverTrigger>
          <PopoverContent side={'left'}>
            <LlmSettingFieldItems
              modelTypes={modelTypes}
              llmOptionTestIdPrefix={optionTestIdPrefix}
            ></LlmSettingFieldItems>
          </PopoverContent>
        </Popover>
      </Select>
    );
  },
);

NextInnerLLMSelect.displayName = 'LLMSelect';

export const NextLLMSelect = memo(NextInnerLLMSelect);
