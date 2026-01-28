import { ExternalToast, toast } from 'sonner';

const configuration: ExternalToast = { duration: 2500, position: 'top-center' };

type MessageOptions = {
  message: string;
  description?: string;
  duration?: number;
};

const message = {
  success: (msg: string) => {
    toast.success(msg, configuration);
  },
  error: (msg: string | MessageOptions, data?: ExternalToast) => {
    let messageText: string;
    let options: ExternalToast = { ...configuration };

    if (typeof msg === 'object') {
      // Object-style call: message.error({ message: '...', description: '...', duration: 3 })
      messageText = msg.message;
      if (msg.description) {
        messageText += `\n${msg.description}`;
      }
      if (msg.duration !== undefined) {
        options.duration = msg.duration * 1000; // Convert to milliseconds
      }
    } else {
      // String-style call: message.error('text', { description: '...' })
      messageText = msg;
      if (data?.description) {
        messageText += `\n${data.description}`;
      }
      options = { ...options, ...data };
    }

    toast.error(messageText, options);
  },
  warning: (msg: string) => {
    toast.warning(msg, configuration);
  },
  info: (msg: string) => {
    toast.info(msg, configuration);
  },
};
export default message;
