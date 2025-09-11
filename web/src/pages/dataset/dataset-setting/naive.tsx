import GraphRagItems from '@/components/parse-configuration/graph-rag-form-fields';
import RaptorFormFields from '@/components/parse-configuration/raptor-form-fields';
import Divider from '@/components/ui/divider';
import {
  ConfigurationFormContainer,
  MainContainer,
} from './configuration-form-container';

export function NaiveConfiguration() {
  return (
    <MainContainer>
      <GraphRagItems className="border-none p-0"></GraphRagItems>
      <Divider />
      <ConfigurationFormContainer>
        <RaptorFormFields></RaptorFormFields>
      </ConfigurationFormContainer>
      <Divider />
    </MainContainer>
  );
}
