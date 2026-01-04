import { LlmModelType } from '@/constants/knowledge';
import { useSetModalState } from '@/hooks/common-hooks';

import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { useSelectLlmOptionsByModelType } from '@/hooks/use-llm-request';
import { useSelectParserList } from '@/hooks/use-user-setting-request';
import kbService from '@/services/knowledge-service';
import { useIsFetching } from '@tanstack/react-query';
import { pick } from 'lodash';
import { useCallback, useEffect, useState } from 'react';
import { UseFormReturn } from 'react-hook-form';
import { useParams, useSearchParams } from 'react-router';
import { z } from 'zod';
import { formSchema } from './form-schema';

// The value that does not need to be displayed in the analysis method Select
const HiddenFields = ['email', 'picture', 'audio'];

export function useSelectChunkMethodList() {
  const parserList = useSelectParserList();

  return parserList.filter((x) => !HiddenFields.some((y) => y === x.value));
}

export function useSelectEmbeddingModelOptions() {
  const allOptions = useSelectLlmOptionsByModelType();
  return allOptions[LlmModelType.Embedding];
}

export function useHasParsedDocument(isEdit?: boolean) {
  const { data: knowledgeDetails } = useFetchKnowledgeBaseConfiguration({
    isEdit,
  });
  return knowledgeDetails.chunk_num > 0;
}

export const useFetchKnowledgeConfigurationOnMount = (
  form: UseFormReturn<z.infer<typeof formSchema>, any, undefined>,
) => {
  const { data: knowledgeDetails, loading } =
    useFetchKnowledgeBaseConfiguration();

  useEffect(() => {
    const parser_config = {
      ...form.formState?.defaultValues?.parser_config,
      ...knowledgeDetails.parser_config,
      raptor: {
        ...form.formState?.defaultValues?.parser_config?.raptor,
        ...knowledgeDetails.parser_config?.raptor,
        use_raptor: true,
      },
      graphrag: {
        ...form.formState?.defaultValues?.parser_config?.graphrag,
        ...knowledgeDetails.parser_config?.graphrag,
        use_graphrag: true,
      },
    };
    const formValues = {
      ...pick({ ...knowledgeDetails, parser_config: parser_config }, [
        'description',
        'name',
        'permission',
        'embd_id',
        'parser_id',
        'language',
        'parser_config',
        'connectors',
        'pagerank',
        'avatar',
      ]),
    } as z.infer<typeof formSchema>;
    form.reset(formValues);
  }, [form, knowledgeDetails]);

  return { knowledgeDetails, loading };
};

export const useSelectKnowledgeDetailsLoading = () =>
  useIsFetching({ queryKey: ['fetchKnowledgeDetail'] }) > 0;

export const useRenameKnowledgeTag = () => {
  const [tag, setTag] = useState<string>('');
  const {
    visible: tagRenameVisible,
    hideModal: hideTagRenameModal,
    showModal: showFileRenameModal,
  } = useSetModalState();

  const handleShowTagRenameModal = useCallback(
    (record: string) => {
      setTag(record);
      showFileRenameModal();
    },
    [showFileRenameModal],
  );

  return {
    initialName: tag,
    tagRenameVisible,
    hideTagRenameModal,
    showTagRenameModal: handleShowTagRenameModal,
  };
};

export const useHandleKbEmbedding = () => {
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const knowledgeBaseId = searchParams.get('id') || id;
  const handleChange = useCallback(
    async ({ embed_id }: { embed_id: string }) => {
      const res = await kbService.checkEmbedding({
        kb_id: knowledgeBaseId,
        embd_id: embed_id,
      });
      return res.data;
    },
    [knowledgeBaseId],
  );
  return {
    handleChange,
  };
};
