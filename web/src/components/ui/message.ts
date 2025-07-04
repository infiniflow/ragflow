import { toast } from 'sonner';

const message = {
  success: (msg: string) => {
    toast.success(msg, {
      position: 'top-center',
      closeButton: false,
    });
  },
  error: (msg: string) => {
    toast.error(msg, {
      position: 'top-center',
      closeButton: false,
    });
  },
  warning: (msg: string) => {
    toast.warning(msg, {
      position: 'top-center',
      closeButton: false,
    });
  },
  info: (msg: string) => {
    toast.info(msg, {
      position: 'top-center',
      closeButton: false,
    });
  },
};
export default message;
