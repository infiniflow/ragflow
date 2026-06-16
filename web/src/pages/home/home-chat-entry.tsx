import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { Button } from '@/components/ui/button';
import {
  ArrowUp,
  Database,
  FileText,
  MessageSquare,
  Sparkles,
} from 'lucide-react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

const chatProfiles = [
  {
    id: 'chat-1',
    title: 'Chat Tesla',
    subject: 'Tesla PDF',
    // type: 'Research',
    dataset: 'Product Knowledge Base',
    prompt: 'Compare competitor pricing and summarize market gaps.',
    lastUsed: 'Last used 8 min ago',
  },
  {
    id: 'chat-2',
    title: 'Chat Company Economy',
    subject: 'Studying',
    // type: 'Support',
    dataset: 'Support Tickets 2026',
    prompt: 'Draft helpful answers from ticket history and policy docs.',
    lastUsed: 'Last choice',
  },
  {
    id: 'chat-3',
    title: 'Chat Studying',
    subject: 'Company',
    // type: 'Sales',
    dataset: 'Sales Playbook',
    prompt: 'Turn account notes into discovery questions and next steps.',
    lastUsed: 'Yesterday',
  },
];

const promptSuggestions = [
  'Summarize my newest dataset',
  'Find insights across documents',
  'Draft a customer support answer',
  'Compare dataset quality',
];

export function HomeChatEntry() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [selectedChatId, setSelectedChatId] = useState(chatProfiles[0].id);
  const [value, setValue] = useState('');
  const selectedChat =
    chatProfiles.find((chat) => chat.id === selectedChatId) ?? chatProfiles[0];

  useEffect(() => {
    const chatId = searchParams.get('chat');
    if (chatProfiles.some((chat) => chat.id === chatId)) {
      setSelectedChatId(chatId as string);
    }
  }, [searchParams]);

  const handleChatChange = (chatId: string) => {
    if (chatId === 'all-chats') {
      navigate('/chats');
      return;
    }

    setSelectedChatId(chatId);
  };

  const startChat = () => {
    if (!value.trim()) return;

    navigate(
      `/chats?chat=${encodeURIComponent(selectedChat.id)}&prompt=${encodeURIComponent(value)}`,
    );
  };

  return (
    <section className="relative mt-8 min-h-[58vh] rounded-[2rem] border border-[var(--border)] bg-[linear-gradient(145deg,var(--surface),var(--surface-soft))] p-4 text-[var(--text)] shadow-[0_24px_80px_rgba(13,27,62,0.10)] dark:shadow-[0_24px_80px_rgba(0,0,0,0.28)] sm:p-6">
      <div className="flex min-w-0 flex-col items-center justify-center py-5">
        <div className="mb-6 flex max-w-3xl flex-col items-center text-center">
          

          <h2 className="max-w-2xl text-3xl font-semibold tracking-normal text-[var(--text)] sm:text-5xl">
            Ask {selectedChat.title}
          </h2>

          <p className="mt-4 max-w-2xl text-base leading-7 text-[var(--text-muted)]">
            This chat uses the {selectedChat.dataset} dataset and keeps its own prompt style.
          </p>
        </div>

        <form
          className="w-full max-w-4xl rounded-3xl border border-[var(--border)] bg-[var(--bg-card)] p-3 shadow-[0_18px_60px_rgba(13,27,62,0.12)] dark:shadow-[0_18px_60px_rgba(0,0,0,0.35)]"
          onSubmit={(event) => {
            event.preventDefault();
            startChat();
          }}
        >
          <div className="mb-3 flex flex-col gap-3 border-b border-[var(--border)] px-2 pb-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <span className="block text-sm font-medium text-[var(--text)]">
                Chat profile
              </span>              
            </div>

            <Select value={selectedChat.id} onValueChange={handleChatChange}>
              <SelectTrigger className="h-10 min-w-64 border-[var(--border-button)] bg-[var(--bg-input)] text-[var(--text)]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent className="bg-[var(--bg-card)]">
                {chatProfiles.map((chat) => (
                  <SelectItem key={chat.id} value={chat.id}>
                    {chat.title}
                  </SelectItem>
                ))}
                <SelectItem value="all-chats">All chats</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex min-h-28 flex-col gap-3">
            <textarea
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder={`Message ${selectedChat.title}...`}
              className="min-h-20 w-full resize-none bg-transparent px-3 py-3 text-base text-[var(--text)] outline-none placeholder:text-[var(--text-disabled)]"
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.shiftKey) {
                  event.preventDefault();
                  startChat();
                }
              }}
            />

            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border)] pt-3">
              <div className="flex flex-wrap gap-2 text-[var(--text-muted)]">
                <span className="inline-flex items-center gap-1 rounded-full bg-[var(--bg-input)] px-3 py-1 text-xs font-medium">
                  <MessageSquare className="size-3.5" />
                  {selectedChat.type}
                </span>
                <span className="inline-flex items-center gap-1 rounded-full bg-[var(--bg-input)] px-3 py-1 text-xs font-medium">
                  <Database className="size-3.5" />
                  {selectedChat.dataset}
                </span>
                <span className="inline-flex items-center gap-1 rounded-full bg-[var(--bg-input)] px-3 py-1 text-xs font-medium">
                  <FileText className="size-3.5" />
                  Files
                </span>
              </div>

              <Button
                aria-label="Start chat"
                className="size-11 rounded-full bg-[var(--button-primary)] p-0 text-[var(--bg-base)] hover:bg-[var(--button-primary-hover)]"
                disabled={!value.trim()}
                size="auto"
                type="submit"
              >
                <ArrowUp className="size-5" />
              </Button>
            </div>
          </div>
        </form>

        <div className="mt-5 grid w-full max-w-4xl gap-2 sm:grid-cols-2 lg:grid-cols-4">
          {promptSuggestions.map((prompt) => (
            <button
              className="rounded-xl border border-[var(--border-button)] bg-[var(--bg-card)] px-4 py-3 text-left text-sm font-medium text-[var(--text-muted)] transition hover:border-[var(--accent-primary)] hover:text-[var(--text)]"
              key={prompt}
              onClick={() => setValue(prompt)}
              type="button"
            >
              <MessageSquare className="mb-2 size-4 text-[var(--accent-primary)]" />
              {prompt}
            </button>
          ))}
        </div>
      </div>
    </section>
  );
}
