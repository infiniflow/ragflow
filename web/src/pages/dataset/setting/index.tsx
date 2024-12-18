import { Card } from '@/components/ui/card';
import AdvancedSettingForm from './advanced-setting-form';
import BasicSettingForm from './basic-setting-form';

export default function DatasetSettings() {
  return (
    <section className="flex flex-col p-8 overflow-auto h-[80vh]">
      <div className="text-3xl font-bold mb-6">Basic settings</div>
      <Card className="border-0 p-6 mb-8 bg-colors-background-inverse-weak">
        <div className="w-2/5">
          <BasicSettingForm></BasicSettingForm>
        </div>
      </Card>

      <div className="text-3xl font-bold mb-6">Advanced settings</div>
      <Card className="border-0 p-6 mb-8 bg-colors-background-inverse-weak">
        <AdvancedSettingForm></AdvancedSettingForm>
      </Card>
    </section>
  );
}
