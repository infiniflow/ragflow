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
  triggerTestId?: string;
  optionTestIdPrefix?: string;
  ownerTenantId?: string;
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
      triggerTestId,
      optionTestIdPrefix,
      ownerTenantId,
    },
    ref,
  ) => {
    const { t } = useTranslation();
    const [isPopoverOpen, setIsPopoverOpen] = useState(false);

    const modelTypes = useMemo(() => {
      if (filter === LlmModelType.Chat) {
        return ['chat'];
      } else if (filter === LlmModelType.Image2text) {
        return ['vision'];
      } else {
        return ['chat', 'vision'];
      }
    }, [filter]);

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
                <LLMLabel value={value} ownerTenantId={ownerTenantId} />
              </SelectValue>
            </SelectTrigger>
          </PopoverTrigger>
          <PopoverContent side={'left'}>
            <LlmSettingFieldItems
              modelTypes={modelTypes}
              llmOptionTestIdPrefix={optionTestIdPrefix}
              ownerTenantId={ownerTenantId}
            ></LlmSettingFieldItems>
          </PopoverContent>
        </Popover>
      </Select>
    );
  },
);

NextInnerLLMSelect.displayName = 'LLMSelect';

export const NextLLMSelect = memo(NextInnerLLMSelect);
