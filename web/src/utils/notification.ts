import { ExternalToast, toast } from 'sonner';

const defaultConfig: ExternalToast = { duration: 4000, position: 'top-right' };

type NotificationOptions = {
  message: string;
  description?: string;
  duration?: number;
};

const notification = {
  success: (options: NotificationOptions) => {
    const messageText = options.description
      ? `${options.message}\n${options.description}`
      : options.message;
    toast.success(messageText, {
      ...defaultConfig,
      duration: options.duration
        ? options.duration * 1000
        : defaultConfig.duration,
    });
  },
  error: (options: NotificationOptions) => {
    const messageText = options.description
      ? `${options.message}\n${options.description}`
      : options.message;
    toast.error(messageText, {
      ...defaultConfig,
      duration: options.duration
        ? options.duration * 1000
        : defaultConfig.duration,
    });
  },
  warning: (options: NotificationOptions) => {
    const messageText = options.description
      ? `${options.message}\n${options.description}`
      : options.message;
    toast.warning(messageText, {
      ...defaultConfig,
      duration: options.duration
        ? options.duration * 1000
        : defaultConfig.duration,
    });
  },
  info: (options: NotificationOptions) => {
    const messageText = options.description
      ? `${options.message}\n${options.description}`
      : options.message;
    toast.info(messageText, {
      ...defaultConfig,
      duration: options.duration
        ? options.duration * 1000
        : defaultConfig.duration,
    });
  },
};

export default notification;
