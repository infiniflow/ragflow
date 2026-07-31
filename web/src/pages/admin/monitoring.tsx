import Spotlight from '@/components/spotlight';
import { Card, CardContent } from '@/components/ui/card';

function AdminMonitoring() {
  return (
    <Card className="!shadow-none relative h-full border-0.5 border-border-button bg-transparent rounded-xl overflow-x-hidden overflow-y-auto">
      <Spotlight />

      <CardContent className="size-full p-0">
        <iframe
          className="size-full"
          src={`${location.protocol}//${location.hostname}:9090/alerts`}
        />
      </CardContent>
    </Card>
  );
}

export default AdminMonitoring;
