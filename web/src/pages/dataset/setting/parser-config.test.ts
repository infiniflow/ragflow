import { normalizeParserConfigForComparison } from './parser-config';

const t = ((key: string) => key) as any;

describe('normalizeParserConfigForComparison', () => {
  const defaults = {
    'Parser:parse': {
      enable_vision_enhancement: true,
      vlm: { llm_id: 'default-vlm' },
    },
  };

  it('resolves an empty parser vision model from the operator defaults', () => {
    const saved = {
      'Parser:parse': {
        enable_vision_enhancement: true,
        vlm: { llm_id: '' },
      },
    };

    const normalized = normalizeParserConfigForComparison(saved, defaults, t);

    expect(normalized['Parser:parse'].vlm.llm_id).toBe('default-vlm');
  });

  it('keeps an explicit parser vision model', () => {
    const saved = {
      'Parser:parse': {
        enable_vision_enhancement: true,
        vlm: { llm_id: 'model-B' },
      },
    };

    const normalized = normalizeParserConfigForComparison(saved, defaults, t);

    expect(normalized['Parser:parse'].vlm.llm_id).toBe('model-B');
  });
});
