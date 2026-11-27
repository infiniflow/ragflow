import { TitleChunkerMethod } from '../../constant/pipeline';
import { transformTitleChunkerParams } from '../../utils';
import { FormSchema } from './index';
import { transformApiResponseToForm } from './hook';

describe('title chunk token cap', () => {
  it.each([128, 512, 8000])('accepts cap %i', (cap) => {
    const values = transformApiResponseToForm({
      method: TitleChunkerMethod.Group,
      chunk_token_cap: cap,
    });
    expect(FormSchema.safeParse(values).success).toBe(true);
  });

  it.each([-1, 0, 1, 127, 8001])('rejects cap %i', (cap) => {
    const values = transformApiResponseToForm({
      method: TitleChunkerMethod.Group,
      chunk_token_cap: cap,
    });
    expect(FormSchema.safeParse(values).success).toBe(false);
  });

  it.each([false, 'many', [], {}])('rejects non-numeric cap %p', (cap) => {
    const values = transformApiResponseToForm({
      method: TitleChunkerMethod.Group,
    });
    expect(
      FormSchema.safeParse({ ...values, hierarchyChunkTokenCap: cap }).success,
    ).toBe(false);
  });

  it('defaults an absent cap to 512', () => {
    const values = transformApiResponseToForm({
      method: TitleChunkerMethod.Hierarchy,
    });
    expect(values.hierarchyChunkTokenCap).toBe(512);
    expect(values.groupChunkTokenCap).toBe(512);
    expect(
      transformTitleChunkerParams(FormSchema.parse(values)).chunk_token_cap,
    ).toBe(512);
  });
});
