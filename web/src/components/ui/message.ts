import { ExternalToast, toast } from 'sonner';

const configuration: ExternalToast = { duration: 2500, position: 'top-center' };

const message = {
  success: (msg: string) => {
    toast.success(msg, configuration);
  },
  error: (msg: string) => {
    toast.error(msg, configuration);
  },
  warning: (msg: string) => {
    toast.warning(msg, configuration);
  },
  info: (msg: string) => {
    toast.info(msg, configuration);
  },
};
export default message;
