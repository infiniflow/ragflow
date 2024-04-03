import {
  useFetchKnowledgeBaseConfiguration,
  useKnowledgeBaseId,
  useSelectKnowledgeDetails,
  useUpdateKnowledge,
} from '@/hooks/knowledgeHook';
import { useFetchLlmList, useSelectLlmOptions } from '@/hooks/llmHooks';
import { useNavigateToDataset } from '@/hooks/routeHook';
import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import {
  useFetchTenantInfo,
  useSelectParserList,
} from '@/hooks/userSettingHook';
import {
  getBase64FromUploadFileList,
  getUploadFileListFromBase64,
} from '@/utils/fileUtil';
import { Form, UploadFile } from 'antd';
import { FormInstance } from 'antd/lib';
import pick from 'lodash/pick';
import { useCallback, useEffect } from 'react';
import { LlmModelType } from '../../constant';

export const useSubmitKnowledgeConfiguration = (form: FormInstance) => {
  const save = useUpdateKnowledge();
  const knowledgeBaseId = useKnowledgeBaseId();
  const submitLoading = useOneNamespaceEffectsLoading('kSModel', ['updateKb']);
  const navigateToDataset = useNavigateToDataset();

  const submitKnowledgeConfiguration = useCallback(async () => {
    const values = await form.validateFields();
    const avatar = await getBase64FromUploadFileList(values.avatar);
    save({
      ...values,
      avatar,
      kb_id: knowledgeBaseId,
    });
    navigateToDataset();
  }, [save, knowledgeBaseId, form, navigateToDataset]);

  return { submitKnowledgeConfiguration, submitLoading, navigateToDataset };
};

export const useFetchKnowledgeConfigurationOnMount = (form: FormInstance) => {
  const knowledgeDetails = useSelectKnowledgeDetails();
  const parserList = useSelectParserList();
  const embeddingModelOptions = useSelectLlmOptions();

  useFetchTenantInfo();
  useFetchKnowledgeBaseConfiguration();
  useFetchLlmList(LlmModelType.Embedding);

  useEffect(() => {
    const fileList: UploadFile[] = getUploadFileListFromBase64(
      knowledgeDetails.avatar,
    );
    form.setFieldsValue({
      ...pick(knowledgeDetails, [
        'description',
        'name',
        'permission',
        'embd_id',
        'parser_id',
        'language',
        'parser_config.chunk_token_num',
      ]),
      avatar: fileList,
    });
  }, [form, knowledgeDetails]);

  return {
    parserList,
    embeddingModelOptions,
    disabled: knowledgeDetails.chunk_num > 0,
  };
};

export const useSelectKnowledgeDetailsLoading = () =>
  useOneNamespaceEffectsLoading('kSModel', ['getKbDetail']);

export const useHandleChunkMethodChange = () => {
  const [form] = Form.useForm();
  const chunkMethod = Form.useWatch('parser_id', form);

  return { form, chunkMethod };
};
