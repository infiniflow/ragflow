import {
  ApiIcon,
  LogOutIcon,
  ModelProviderIcon,
  PasswordIcon,
  ProfileIcon,
  TeamIcon,
} from '@/assets/icon/next-icon';
import { IconFont } from '@/components/icon-font';
import { LLMFactory } from '@/constants/llm';
import { UserSettingRouteKey } from '@/constants/setting';
import { MonitorOutlined } from '@ant-design/icons';

export const UserSettingIconMap = {
  [UserSettingRouteKey.Profile]: <ProfileIcon />,
  [UserSettingRouteKey.Password]: <PasswordIcon />,
  [UserSettingRouteKey.Model]: <ModelProviderIcon />,
  [UserSettingRouteKey.System]: <MonitorOutlined style={{ fontSize: 24 }} />,
  [UserSettingRouteKey.Team]: <TeamIcon />,
  [UserSettingRouteKey.Logout]: <LogOutIcon />,
  [UserSettingRouteKey.Api]: <ApiIcon />,
  [UserSettingRouteKey.MCP]: (
    <IconFont name="mcp" className="size-6"></IconFont>
  ),
};

export * from '@/constants/setting';

export const LocalLlmFactories = [
  LLMFactory.Ollama,
  LLMFactory.Xinference,
  LLMFactory.LocalAI,
  LLMFactory.LMStudio,
  LLMFactory.OpenAiAPICompatible,
  LLMFactory.TogetherAI,
  LLMFactory.Replicate,
  LLMFactory.OpenRouter,
  LLMFactory.HuggingFace,
  LLMFactory.GPUStack,
  LLMFactory.ModelScope,
  LLMFactory.VLLM,
];

export enum TenantRole {
  Owner = 'owner',
  Invite = 'invite',
  Normal = 'normal',
}
