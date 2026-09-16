import { Form } from '@/components/ui/form';
import { TooltipProvider } from '@/components/ui/tooltip';
import en from '@/locales/en';
import zh from '@/locales/zh';
import {
  SimilaritySliderFormField,
  similarityThresholdTipKey,
  similarityWeightTipKey,
} from '../index';
import i18next from 'i18next';
import { render, screen } from '@testing-library/react';
import { useForm } from 'react-hook-form';

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
if (typeof globalThis.ResizeObserver === 'undefined') {
  (globalThis as unknown as Record<string, unknown>).ResizeObserver =
    ResizeObserverStub;
}

interface HarnessProps {
  rerankId?: string;
  isRerankEnabled?: boolean;
}

function Harness({ rerankId = '', isRerankEnabled }: HarnessProps) {
  const form = useForm({
    defaultValues: {
      similarity_threshold: 0.2,
      keywords_similarity_weight: 0.7,
      rerank_id: rerankId,
    },
  });
  return (
    <TooltipProvider delayDuration={0}>
      <Form {...form}>
        <SimilaritySliderFormField
          similarityWeightName="keywords_similarity_weight"
          similarityWeightType="keyword"
          isTooltipShown
          isRerankEnabled={isRerankEnabled}
        ></SimilaritySliderFormField>
      </Form>
    </TooltipProvider>
  );
}

beforeAll(() =>
  i18next.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en: { translation: en.translation } },
  }),
);

describe('SimilaritySliderFormField', () => {
  it('labels the weighted legs vector and full-text without a rerank model', () => {
    render(<Harness></Harness>);

    expect(screen.getByText('vector')).toBeInTheDocument();
    expect(screen.getByText('full-text')).toBeInTheDocument();
    expect(screen.queryByText('rerank')).not.toBeInTheDocument();
  });

  it('labels the second leg rerank once a rerank model is selected', () => {
    render(<Harness rerankId="rerank-model-id"></Harness>);

    expect(screen.getByText('rerank')).toBeInTheDocument();
    expect(screen.getByText('full-text')).toBeInTheDocument();
    expect(screen.queryByText('vector')).not.toBeInTheDocument();
  });

  it('lets the caller override the watched rerank field', () => {
    render(
      <Harness rerankId="rerank-model-id" isRerankEnabled={false}></Harness>,
    );

    expect(screen.getByText('vector')).toBeInTheDocument();
  });

  it('selects the rerank-aware tooltip variants per scoring mode', () => {
    expect(similarityThresholdTipKey(false)).toBe('similarityThresholdTip');
    expect(similarityThresholdTipKey(true)).toBe(
      'similarityThresholdTipWithRerank',
    );
    expect(similarityWeightTipKey(false, false)).toBe(
      'keywordSimilarityWeightTip',
    );
    expect(similarityWeightTipKey(false, true)).toBe(
      'vectorSimilarityWeightTip',
    );
    expect(similarityWeightTipKey(true, false)).toBe(
      'keywordSimilarityWeightTipWithRerank',
    );
    expect(similarityWeightTipKey(true, true)).toBe(
      'vectorSimilarityWeightTipWithRerank',
    );
  });
});

describe('similarity slider locales', () => {
  const rerankAwareTipKeys = [
    'similarityThresholdTip',
    'vectorSimilarityWeightTip',
    'keywordSimilarityWeightTip',
  ] as const;

  it.each([
    ['en', en.translation.knowledgeDetails],
    ['zh', zh.translation.knowledgeDetails],
  ])('ships rerank-aware tooltip variants for %s', (_lang, details) => {
    const tips = details as unknown as Record<string, string>;
    for (const key of rerankAwareTipKeys) {
      expect(tips[`${key}WithRerank`]).toBeTruthy();
      expect(tips[`${key}WithRerank`]).not.toBe(tips[key]);
    }
  });
});
