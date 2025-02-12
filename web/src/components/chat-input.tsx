import { useEventListener } from 'ahooks';
import { Mic, Paperclip, Send } from 'lucide-react';
import { useRef, useState } from 'react';
import { Button } from './ui/button';
import { Textarea } from './ui/textarea';

export function ChatInput() {
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [textareaHeight, setTextareaHeight] = useState<number>(40);

  useEventListener(
    'keydown',
    (ev) => {
      if (ev.shiftKey && ev.code === 'Enter') {
        setTextareaHeight((h) => {
          return h + 10;
        });
      }
    },
    {
      target: textareaRef,
    },
  );

  return (
    <section className="flex items-end bg-colors-background-neutral-strong px-4 py-3 rounded-xl m-8">
      <Button variant={'icon'} className="w-10 h-10">
        <Mic />
      </Button>
      <Textarea
        ref={textareaRef}
        placeholder="Tell us a little bit about yourself "
        className="resize-none focus-visible:ring-0 focus-visible:ring-offset-0 bg-transparent border-none min-h-0 max-h-20"
        style={{ height: textareaHeight }}
      />
      <div className="flex gap-2">
        <Button variant={'icon'} size={'icon'}>
          <Paperclip />
        </Button>
        <Button variant={'tertiary'} size={'icon'}>
          <Send />
        </Button>
      </div>
    </section>
  );
}
