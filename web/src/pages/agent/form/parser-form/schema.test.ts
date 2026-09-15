import { FileType, ParserFields } from '../../constant/pipeline';
import { FormSchema } from './schema';

describe('parser FormSchema', () => {
  it('accepts video and audio setups without a model', () => {
    const result = FormSchema.safeParse({
      setups: [
        { fileFormat: FileType.Video, output_format: 'text' },
        { fileFormat: FileType.Audio, output_format: 'text' },
      ],
    });
    expect(result.success).toBe(true);
  });

  it('requires at least one file type', () => {
    const result = FormSchema.safeParse({ setups: [] });
    expect(result.success).toBe(false);
  });

  it('still requires fields for email', () => {
    const result = FormSchema.safeParse({
      setups: [{ fileFormat: FileType.Email, output_format: 'text' }],
    });
    expect(result.success).toBe(false);

    const withFields = FormSchema.safeParse({
      setups: [
        {
          fileFormat: FileType.Email,
          output_format: 'text',
          fields: Object.values(ParserFields),
        },
      ],
    });
    expect(withFields.success).toBe(true);
  });
});
