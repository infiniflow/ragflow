import { TitleChunkerMethod } from '../../constant/pipeline';
import { transformTitleChunkerParams } from '../../utils';
import { FormSchema } from './index';
import { transformApiResponseToForm } from './hook';
import {
  transformApiConfigToForm,
  transformFormConfigToApi,
} from '@/utils/pipeline-operator';

describe('title/manual media context configuration', () => {
  it.each([TitleChunkerMethod.Group, TitleChunkerMethod.Hierarchy])(
    'defaults omitted windows to zero for %s',
    (method) => {
      const values = transformApiResponseToForm({ method });
      const saved = transformTitleChunkerParams(FormSchema.parse(values));
      expect(saved.table_context_size).toBe(0);
      expect(saved.image_context_size).toBe(0);
    },
  );

  it.each([
    [0, 0],
    [32, 0],
    [0, 64],
    [32, 64],
    [256, 256],
  ])(
    'round trips table=%i and image=%i across method changes',
    (table, image) => {
      const original = {
        method: TitleChunkerMethod.Hierarchy,
        table_context_size: table,
        image_context_size: image,
      };
      const values = FormSchema.parse(transformApiResponseToForm(original));
      const saved = transformTitleChunkerParams(values);
      const reloaded = transformApiResponseToForm(
        JSON.parse(JSON.stringify(saved)),
      );
      expect(reloaded.table_context_size).toBe(table);
      expect(reloaded.image_context_size).toBe(image);
      const group = transformTitleChunkerParams(
        FormSchema.parse({ ...reloaded, method: TitleChunkerMethod.Group }),
      );
      expect(group.table_context_size).toBe(table);
      expect(group.image_context_size).toBe(image);
    },
  );

  it.each(['table_context_size', 'image_context_size'])(
    'rejects negative, fractional and nonnumeric %s values',
    (field) => {
      for (const value of [-1, 0.5, NaN, Infinity, '32', false, null]) {
        const values = transformApiResponseToForm({
          method: TitleChunkerMethod.Group,
        });
        expect(
          FormSchema.safeParse({ ...values, [field]: value }).success,
        ).toBe(false);
      }
    },
  );

  it.each(['TitleChunker', 'ManualChunker'])(
    'preserves both windows through the actual %s operator bridge',
    (operator) => {
      const loaded = transformApiConfigToForm(operator, {
        method: 'group',
        hierarchy: 0,
        levels: [['^# ']],
        table_context_size: 32,
        image_context_size: 64,
      });
      const parsed = FormSchema.parse(transformApiResponseToForm(loaded));
      const saved = transformFormConfigToApi(operator, parsed);
      const reloaded = transformApiConfigToForm(
        operator,
        JSON.parse(JSON.stringify(saved)),
      );
      expect(reloaded.table_context_size).toBe(32);
      expect(reloaded.image_context_size).toBe(64);
      if (operator === 'ManualChunker') {
        expect(saved).not.toHaveProperty('chunk_token_cap');
        // The existing bridge writes false; Manual ignores this setting.
        expect(saved.include_heading_content).toBe(false);
      }
    },
  );
});
