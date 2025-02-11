import ChatBasicSetting from './chat-basic-settings';
import { ChatModelSettings } from './chat-model-settings';
import { ChatPromptEngine } from './chat-prompt-engine';

export function AppSettings() {
  return (
    <section className="p-6 w-[500px] max-w-[25%]">
      <div className="text-2xl font-bold mb-4 text-colors-text-neutral-strong">
        App settings
      </div>
      <ChatBasicSetting></ChatBasicSetting>
      <ChatPromptEngine></ChatPromptEngine>
      <ChatModelSettings></ChatModelSettings>
    </section>
  );
}
