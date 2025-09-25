import { toast } from 'sonner';

const duration = { duration: 1500 };

const message = {
  success: (msg: string) => {
    toast.success(msg, {
      position: 'top-center',
      closeButton: false,
      ...duration,
    });
  },
  error: (msg: string) => {
    toast.error(msg, {
      position: 'top-center',
      closeButton: false,
      ...duration,
    });
  },
  warning: (msg: string) => {
    toast.warning(msg, {
      position: 'top-center',
      closeButton: false,
      ...duration,
    });
  },
  info: (msg: string) => {
    toast.info(msg, {
      position: 'top-center',
      closeButton: false,
      ...duration,
    });
  },
};
export default message;
