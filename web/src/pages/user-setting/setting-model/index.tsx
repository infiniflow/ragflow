import Spotlight from '@/components/spotlight';
import { LLMFactory } from '@/constants/llm';
import { LlmItem, useFetchMyLlmListDetailed } from '@/hooks/llm-hooks';
import { useCallback, useMemo } from 'react';
import { isLocalLlmFactory } from '../utils';
import SystemSetting from './components/system-setting';
import { AvailableModels } from './components/un-add-model';
import { UsedModel } from './components/used-model';
import {
  useSubmitApiKey,
  useSubmitAzure,
  useSubmitBedrock,
  useSubmitFishAudio,
  useSubmitGoogle,
  useSubmitHunyuan,
  useSubmitOllama,
  useSubmitSpark,
  useSubmitSystemModelSetting,
  useSubmitTencentCloud,
  useSubmitVolcEngine,
  useSubmityiyan,
} from './hooks';
import ApiKeyModal from './modal/api-key-modal';
import AzureOpenAIModal from './modal/azure-openai-modal';
import BedrockModal from './modal/bedrock-modal';
import FishAudioModal from './modal/fish-audio-modal';
import GoogleModal from './modal/google-modal';
import HunyuanModal from './modal/hunyuan-modal';
import TencentCloudModal from './modal/next-tencent-modal';
import OllamaModal from './modal/ollama-modal';
import SparkModal from './modal/spark-modal';
import VolcEngineModal from './modal/volcengine-modal';
import YiyanModal from './modal/yiyan-modal';
const ModelProviders = () => {
  const { saveSystemModelSettingLoading, onSystemSettingSavingOk } =
    useSubmitSystemModelSetting();
  const { data: detailedLlmList } = useFetchMyLlmListDetailed();
  const {
    saveApiKeyLoading,
    initialApiKey,
    llmFactory,
    editMode,
    onApiKeySavingOk,
    apiKeyVisible,
    hideApiKeyModal,
    showApiKeyModal,
  } = useSubmitApiKey();
  const {
    llmAddingVisible,
    hideLlmAddingModal,
    showLlmAddingModal,
    onLlmAddingOk,
    llmAddingLoading,
    editMode: llmEditMode,
    initialValues: llmInitialValues,
    selectedLlmFactory,
  } = useSubmitOllama();

  const {
    volcAddingVisible,
    hideVolcAddingModal,
    showVolcAddingModal,
    onVolcAddingOk,
    volcAddingLoading,
  } = useSubmitVolcEngine();

  const {
    HunyuanAddingVisible,
    hideHunyuanAddingModal,
    showHunyuanAddingModal,
    onHunyuanAddingOk,
    HunyuanAddingLoading,
  } = useSubmitHunyuan();

  const {
    GoogleAddingVisible,
    hideGoogleAddingModal,
    showGoogleAddingModal,
    onGoogleAddingOk,
    GoogleAddingLoading,
  } = useSubmitGoogle();

  const {
    TencentCloudAddingVisible,
    hideTencentCloudAddingModal,
    showTencentCloudAddingModal,
    onTencentCloudAddingOk,
    TencentCloudAddingLoading,
  } = useSubmitTencentCloud();

  const {
    SparkAddingVisible,
    hideSparkAddingModal,
    showSparkAddingModal,
    onSparkAddingOk,
    SparkAddingLoading,
  } = useSubmitSpark();

  const {
    yiyanAddingVisible,
    hideyiyanAddingModal,
    showyiyanAddingModal,
    onyiyanAddingOk,
    yiyanAddingLoading,
  } = useSubmityiyan();

  const {
    FishAudioAddingVisible,
    hideFishAudioAddingModal,
    showFishAudioAddingModal,
    onFishAudioAddingOk,
    FishAudioAddingLoading,
  } = useSubmitFishAudio();

  const {
    bedrockAddingLoading,
    onBedrockAddingOk,
    bedrockAddingVisible,
    hideBedrockAddingModal,
    showBedrockAddingModal,
  } = useSubmitBedrock();

  const {
    AzureAddingVisible,
    hideAzureAddingModal,
    showAzureAddingModal,
    onAzureAddingOk,
    AzureAddingLoading,
  } = useSubmitAzure();

  const ModalMap = useMemo(
    () => ({
      [LLMFactory.Bedrock]: showBedrockAddingModal,
      [LLMFactory.VolcEngine]: showVolcAddingModal,
      [LLMFactory.TencentHunYuan]: showHunyuanAddingModal,
      [LLMFactory.XunFeiSpark]: showSparkAddingModal,
      [LLMFactory.BaiduYiYan]: showyiyanAddingModal,
      [LLMFactory.FishAudio]: showFishAudioAddingModal,
      [LLMFactory.TencentCloud]: showTencentCloudAddingModal,
      [LLMFactory.GoogleCloud]: showGoogleAddingModal,
      [LLMFactory.AzureOpenAI]: showAzureAddingModal,
    }),
    [
      showBedrockAddingModal,
      showVolcAddingModal,
      showHunyuanAddingModal,
      showTencentCloudAddingModal,
      showSparkAddingModal,
      showyiyanAddingModal,
      showFishAudioAddingModal,
      showGoogleAddingModal,
      showAzureAddingModal,
    ],
  );

  const handleAddModel = useCallback(
    (llmFactory: string) => {
      console.log('handleAddModel', llmFactory);
      if (isLocalLlmFactory(llmFactory)) {
        showLlmAddingModal(llmFactory);
      } else if (llmFactory in ModalMap) {
        ModalMap[llmFactory as keyof typeof ModalMap]();
      } else {
        showApiKeyModal({ llm_factory: llmFactory });
      }
    },
    [showApiKeyModal, showLlmAddingModal, ModalMap],
  );

  const handleEditModel = useCallback(
    (model: any, factory: LlmItem) => {
      if (factory) {
        const detailedFactory = detailedLlmList[factory.name];
        const detailedModel = detailedFactory?.llm?.find(
          (m: any) => m.name === model.name,
        );

        const editData = {
          llm_factory: factory.name,
          llm_name: model.name,
          model_type: model.type,
        };

        if (isLocalLlmFactory(factory.name)) {
          showLlmAddingModal(factory.name, true, editData, detailedModel);
        } else if (factory.name in ModalMap) {
          ModalMap[factory.name as keyof typeof ModalMap]();
        } else {
          showApiKeyModal(editData, true);
        }
      }
    },
    [showApiKeyModal, showLlmAddingModal, ModalMap, detailedLlmList],
  );
  return (
    <div className="flex w-full border-[0.5px] border-border-button rounded-lg relative ">
      <Spotlight />
      <section className="flex flex-col gap-4 w-3/5 px-5 border-r-[0.5px] border-border-button overflow-auto scrollbar-auto">
        <SystemSetting
          onOk={onSystemSettingSavingOk}
          loading={saveSystemModelSettingLoading}
        />
        <UsedModel
          handleAddModel={handleAddModel}
          handleEditModel={handleEditModel}
        />
      </section>
      <section className="flex flex-col w-2/5 overflow-auto scrollbar-auto">
        <AvailableModels handleAddModel={handleAddModel} />
      </section>
      <ApiKeyModal
        visible={apiKeyVisible}
        hideModal={hideApiKeyModal}
        loading={saveApiKeyLoading}
        initialValue={initialApiKey}
        editMode={editMode}
        onOk={onApiKeySavingOk}
        llmFactory={llmFactory}
      ></ApiKeyModal>
      <OllamaModal
        visible={llmAddingVisible}
        hideModal={hideLlmAddingModal}
        onOk={onLlmAddingOk}
        loading={llmAddingLoading}
        editMode={llmEditMode}
        initialValues={llmInitialValues}
        llmFactory={selectedLlmFactory}
      ></OllamaModal>
      <VolcEngineModal
        visible={volcAddingVisible}
        hideModal={hideVolcAddingModal}
        onOk={onVolcAddingOk}
        loading={volcAddingLoading}
        llmFactory={LLMFactory.VolcEngine}
      ></VolcEngineModal>
      <HunyuanModal
        visible={HunyuanAddingVisible}
        hideModal={hideHunyuanAddingModal}
        onOk={onHunyuanAddingOk}
        loading={HunyuanAddingLoading}
        llmFactory={LLMFactory.TencentHunYuan}
      ></HunyuanModal>
      <GoogleModal
        visible={GoogleAddingVisible}
        hideModal={hideGoogleAddingModal}
        onOk={onGoogleAddingOk}
        loading={GoogleAddingLoading}
        llmFactory={LLMFactory.GoogleCloud}
      ></GoogleModal>
      <TencentCloudModal
        visible={TencentCloudAddingVisible}
        hideModal={hideTencentCloudAddingModal}
        onOk={onTencentCloudAddingOk}
        loading={TencentCloudAddingLoading}
        llmFactory={LLMFactory.TencentCloud}
      ></TencentCloudModal>
      <SparkModal
        visible={SparkAddingVisible}
        hideModal={hideSparkAddingModal}
        onOk={onSparkAddingOk}
        loading={SparkAddingLoading}
        llmFactory={LLMFactory.XunFeiSpark}
      ></SparkModal>
      <YiyanModal
        visible={yiyanAddingVisible}
        hideModal={hideyiyanAddingModal}
        onOk={onyiyanAddingOk}
        loading={yiyanAddingLoading}
        llmFactory={LLMFactory.BaiduYiYan}
      ></YiyanModal>
      <FishAudioModal
        visible={FishAudioAddingVisible}
        hideModal={hideFishAudioAddingModal}
        onOk={onFishAudioAddingOk}
        loading={FishAudioAddingLoading}
        llmFactory={LLMFactory.FishAudio}
      ></FishAudioModal>
      <BedrockModal
        visible={bedrockAddingVisible}
        hideModal={hideBedrockAddingModal}
        onOk={onBedrockAddingOk}
        loading={bedrockAddingLoading}
        llmFactory={LLMFactory.Bedrock}
      ></BedrockModal>
      <AzureOpenAIModal
        visible={AzureAddingVisible}
        hideModal={hideAzureAddingModal}
        onOk={onAzureAddingOk}
        loading={AzureAddingLoading}
        llmFactory={LLMFactory.AzureOpenAI}
      ></AzureOpenAIModal>
    </div>
  );
};
export default ModelProviders;
