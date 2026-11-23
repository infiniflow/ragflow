import en from '@/locales/en';
import zh from '@/locales/zh';
import { get } from 'lodash';
import {
  buildSearchSettingFormSchema,
  SearchSettingFormSchema,
  type SearchSettingFormData,
} from './search-setting-hooks';

const ValidFormData: SearchSettingFormData = {
  name: 'Search',
  search_config: {
    kb_ids: ['dataset-id'],
    keywords_similarity_weight: 0.7,
    web_search: false,
    similarity_threshold: 0.2,
    use_kg: false,
    rerank_id: 'rerank-id',
    use_rerank: true,
    rerank_candidates_count: 100,
    summary: true,
    llm_setting: { llm_id: 'llm-id' },
    related_search: true,
    query_mindmap: true,
    doc_ids: [],
    chat_id: '',
    highlight: false,
    keyword: false,
    cross_languages: [],
  },
};

describe('Search language settings', () => {
  it('keeps the selected retrieval languages in the save payload', () => {
    const search_config = {
      ...ValidFormData.search_config,
      cross_languages: ['German', 'French'],
    };
    const parsed = SearchSettingFormSchema.parse({
      ...ValidFormData,
      search_config,
    });

    expect(parsed.search_config.cross_languages).toEqual(['German', 'French']);
  });

  it('defaults to the original query when an older search has no languages', () => {
    const search_config = {
      ...ValidFormData.search_config,
      cross_languages: undefined,
    };
    const parsed = SearchSettingFormSchema.parse({
      ...ValidFormData,
      search_config,
    });

    expect(parsed.search_config.cross_languages).toEqual([]);
  });
});

const InvalidFormData: SearchSettingFormData = {
  ...ValidFormData,
  name: '',
  search_config: {
    ...ValidFormData.search_config,
    kb_ids: [],
    rerank_id: '',
    llm_setting: { llm_id: '' },
  },
};

describe('buildSearchSettingFormSchema', () => {
  it.each([
    ['Chinese', zh.translation],
    ['English', en.translation],
  ])('uses %s required-field messages', (_, translation) => {
    const formSchema = buildSearchSettingFormSchema(
      (key) => get(translation, key) as string,
    );

    const result = formSchema.safeParse(InvalidFormData);

    expect(result.success).toBe(false);
    if (result.success) return;

    const messages = Object.fromEntries(
      result.error.issues.map((issue) => [issue.path.join('.'), issue.message]),
    );
    expect(messages).toMatchObject({
      name: translation.search.nameRequired,
      'search_config.kb_ids': translation.search.datasetRequired,
      'search_config.rerank_id': translation.search.rerankModelRequired,
      'search_config.llm_setting.llm_id': translation.search.modelRequired,
    });
  });
});
