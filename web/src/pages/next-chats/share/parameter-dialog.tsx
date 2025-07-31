import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useFetchExternalAgentInputs } from '@/hooks/use-agent-request';
import { IModalProps } from '@/interfaces/common';
import DebugContent from '@/pages/agent/debug-content';
import { buildBeginInputListFromObject } from '@/pages/agent/form/begin-form/utils';

interface IProps extends IModalProps<any> {
  ok(parameters: any[]): void;
}
export function ParameterDialog({ hideModal, ok }: IProps) {
  const { data } = useFetchExternalAgentInputs();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Parameter</DialogTitle>
        </DialogHeader>
        <DebugContent
          parameters={buildBeginInputListFromObject(data)}
          ok={ok}
          isNext={false}
          btnText={'Submit'}
        ></DebugContent>
      </DialogContent>
    </Dialog>
  );
}
