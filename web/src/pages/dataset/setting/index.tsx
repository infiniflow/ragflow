import { Card, CardContent } from '@/components/ui/card';
import AdvancedSettingForm from './advanced-setting-form';
import BasicSettingForm from './basic-setting-form';

export default function DatasetSettings() {
  return (
    <section className="p-8 overflow-y-scroll max-h-[90vh]">
      <div className="text-3xl font-bold pb-6">Basic settings</div>
      <Card className="border-0 p-6 bg-colors-background-inverse-weak">
        <CardContent>
          <div className="w-2/5">
            <BasicSettingForm></BasicSettingForm>
          </div>
        </CardContent>
      </Card>

      <div className="text-3xl font-bold pb-6 pt-8">Advanced settings</div>
      <Card className="border-0 p-6 bg-colors-background-inverse-weak">
        <CardContent>
          <AdvancedSettingForm></AdvancedSettingForm>
        </CardContent>
      </Card>
    </section>
  );
}
