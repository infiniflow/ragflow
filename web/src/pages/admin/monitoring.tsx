import { Card, CardContent } from '@/components/ui/card';

function AdminMonitoring() {
  return (
    <Card className="!shadow-none h-full border border-border-button bg-transparent rounded-xl overflow-x-hidden overflow-y-auto">
      <CardContent className="size-full p-0">
        <iframe />
      </CardContent>
    </Card>
  );
}

export default AdminMonitoring;
