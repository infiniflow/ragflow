import { normalizeParserConfigForComparison } from './parser-config';

const t = ((key: string) => key) as any;

describe('normalizeParserConfigForComparison', () => {
  const defaults = {
    'Parser:PaperTrail': {
      enable_vision_enhancement: true,
      vlm: { llm_id: 'default-vlm' },
    },
  };

  it('resolves an empty parser vision model from the operator defaults', () => {
    const saved = {
      'Parser:PaperTrail': {
        enable_vision_enhancement: true,
        vlm: { llm_id: '' },
      },
    };

    const normalized = normalizeParserConfigForComparison(saved, defaults, t);

    expect(normalized['Parser:PaperTrail'].vlm.llm_id).toBe('default-vlm');
  });

  it('keeps an explicit parser vision model', () => {
    const saved = {
      'Parser:PaperTrail': {
        enable_vision_enhancement: true,
        vlm: { llm_id: 'model-B' },
        visio: {},
      },
    };

    const normalized = normalizeParserConfigForComparison(saved, defaults, t);

    expect(normalized['Parser:PaperTrail'].vlm.llm_id).toBe('model-B');
  });
});
