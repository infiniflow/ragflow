import { useFormContext, useWatch } from 'react-hook-form';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from './ui/form';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './ui/select';
import { Switch } from './ui/switch';

// MinerU OCR language options with human-readable labels
const MINERU_LANG_OPTIONS = [
  { value: 'ch', label: 'Chinese (Simplified)' },
  { value: 'en', label: 'English' },
  { value: 'cyrillic', label: 'Cyrillic (Russian, Ukrainian, etc.)' },
  { value: 'latin', label: 'Latin (French, German, Spanish, etc.)' },
  { value: 'korean', label: 'Korean' },
  { value: 'japan', label: 'Japanese' },
  { value: 'arabic', label: 'Arabic' },
  { value: 'th', label: 'Thai' },
  { value: 'el', label: 'Greek' },
  { value: 'devanagari', label: 'Hindi (Devanagari)' },
  { value: 'ta', label: 'Tamil' },
  { value: 'te', label: 'Telugu' },
  { value: 'ka', label: 'Georgian/Kannada' },
  { value: 'chinese_cht', label: 'Chinese (Traditional)' },
];

/**
 * Check if the current layout recognizer is MinerU
 */
function useIsMineruSelected() {
  const form = useFormContext();
  const layoutRecognize = useWatch({
    control: form.control,
    name: 'parser_config.layout_recognize',
  });

  // MinerU models have format like "model-name@MinerU"
  return (
    typeof layoutRecognize === 'string' &&
    (layoutRecognize.toLowerCase().includes('mineru') ||
      layoutRecognize.toLowerCase().endsWith('@mineru'))
  );
}

export function MineruConfigFormField() {
  const form = useFormContext();
  const isMineruSelected = useIsMineruSelected();

  if (!isMineruSelected) {
    return null;
  }

  return (
    <div className="space-y-4 p-4 border rounded-lg bg-muted/50">
      <div className="text-sm font-medium text-foreground">
        MinerU OCR Settings
      </div>

      {/* MinerU Language Selection */}
      <FormField
        control={form.control}
        name="parser_config.mineru_lang"
        render={({ field }) => (
          <FormItem className="items-center space-y-0">
            <div className="flex items-center">
              <FormLabel className="text-sm text-text-secondary whitespace-wrap w-1/3">
                OCR Language
              </FormLabel>
              <div className="w-2/3">
                <FormControl>
                  <Select
                    value={field.value || 'latin'}
                    onValueChange={field.onChange}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="Select language" />
                    </SelectTrigger>
                    <SelectContent>
                      {MINERU_LANG_OPTIONS.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {option.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FormControl>
              </div>
            </div>
            <FormMessage />
          </FormItem>
        )}
      />

      {/* Formula Recognition Toggle */}
      <FormField
        control={form.control}
        name="parser_config.mineru_formula_enable"
        render={({ field }) => (
          <FormItem className="items-center space-y-0">
            <div className="flex items-center">
              <FormLabel className="text-sm text-text-secondary whitespace-wrap w-1/3">
                Formula Recognition
              </FormLabel>
              <div className="w-2/3">
                <FormControl>
                  <Switch
                    checked={field.value ?? true}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </div>
            </div>
            <div className="text-xs text-muted-foreground mt-1 ml-[33.33%]">
              Disable for Cyrillic/stylized fonts to avoid incorrect LaTeX
              conversion
            </div>
            <FormMessage />
          </FormItem>
        )}
      />

      {/* Table Recognition Toggle */}
      <FormField
        control={form.control}
        name="parser_config.mineru_table_enable"
        render={({ field }) => (
          <FormItem className="items-center space-y-0">
            <div className="flex items-center">
              <FormLabel className="text-sm text-text-secondary whitespace-wrap w-1/3">
                Table Recognition
              </FormLabel>
              <div className="w-2/3">
                <FormControl>
                  <Switch
                    checked={field.value ?? true}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </div>
            </div>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  );
}
