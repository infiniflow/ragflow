import { Button } from '@/components/ui/button';
import { Database, FileText, MessageSquare, Upload } from 'lucide-react';
import { useNavigate } from 'react-router';

const recentChats = [
  {
    title: 'Dataset onboarding checklist',
    description: 'Summarized upload rules and missing metadata fields.',
    time: '12 min ago',
  },
  {
    title: 'Customer feedback insights',
    description: 'Grouped recurring issues from support notes.',
    time: 'Yesterday',
  },
  {
    title: 'Policy document Q&A',
    description: 'Pulled answers from the internal compliance docs.',
    time: 'Jun 14',
  },
];

const recentDatasets = [
  {
    name: 'Product Knowledge Base',
    files: '128 files',
    status: 'Ready',
  },
  {
    name: 'Support Tickets 2026',
    files: '42 files',
    status: 'Indexing',
  },
  {
    name: 'Sales Playbook',
    files: '18 files',
    status: 'Ready',
  },
];

export function HomeHero() {
  const navigate = useNavigate();

  return (
    <section className="mt-8 flex flex-col gap-4 text-[var(--text)] xl:flex-row">
      <div className="flex min-w-0 flex-1 flex-col rounded-3xl border border-[var(--border)] bg-[var(--bg-card)] p-5 shadow-[0_16px_50px_rgba(13,27,62,0.08)] dark:shadow-[0_16px_50px_rgba(0,0,0,0.22)]">
        <div className="mb-4 flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <span className="inline-flex size-10 items-center justify-center rounded-2xl bg-[var(--bg-input)] text-[var(--button-primary)]">
              <MessageSquare className="size-5" />
            </span>
            <div>
              <h2 className="text-lg font-semibold">Recent chats</h2>
              <p className="text-sm text-[var(--text-muted)]">Dummy chat history preview</p>
            </div>
          </div>

          <Button
            className="border-[var(--border-button)] bg-[var(--bg-input)] text-[var(--text)] hover:bg-[var(--bg-component)]"
            onClick={() => navigate('/chats')}
            size="sm"
            variant="outline"
          >
            View all
          </Button>
        </div>

        <div className="flex flex-col gap-3">
          {recentChats.map((chat) => (
            <button
              className="group flex min-h-24 items-start gap-3 rounded-2xl border border-[var(--border-button)] bg-[var(--bg-input)] p-4 text-left transition hover:border-[var(--accent-primary)] hover:bg-[var(--bg-component)]"
              key={chat.title}
              onClick={() => navigate('/chats')}
              type="button"
            >
              <span className="mt-1 inline-flex size-9 shrink-0 items-center justify-center rounded-xl bg-[var(--bg-card)] text-[var(--accent-primary)]">
                <MessageSquare className="size-4" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium text-[var(--text)]">
                  {chat.title}
                </span>
                <span className="mt-1 line-clamp-2 block text-sm leading-6 text-[var(--text-muted)]">
                  {chat.description}
                </span>
              </span>
              <span className="shrink-0 text-xs text-[var(--text-disabled)]">
                {chat.time}
              </span>
            </button>
          ))}
        </div>
      </div>

      <div className="flex min-w-0 flex-1 flex-col rounded-3xl border border-[var(--border)] bg-[var(--bg-card)] p-5 shadow-[0_16px_50px_rgba(13,27,62,0.08)] dark:shadow-[0_16px_50px_rgba(0,0,0,0.22)]">
        <div className="mb-4 flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <span className="inline-flex size-10 items-center justify-center rounded-2xl bg-[var(--bg-input)] text-[var(--button-primary)]">
              <Database className="size-5" />
            </span>
            <div>
              <h2 className="text-lg font-semibold">Datasets</h2>
              <p className="text-sm text-[var(--text-muted)]">Dummy dataset status preview</p>
            </div>
          </div>

          <Button
            className="border-[var(--border-button)] bg-[var(--bg-input)] text-[var(--text)] hover:bg-[var(--bg-component)]"
            onClick={() => navigate('/datasets')}
            size="sm"
            variant="outline"
          >
            View all
          </Button>
        </div>

        <div className="flex flex-col gap-3">
          {recentDatasets.map((dataset) => (
            <button
              className="flex min-h-24 items-center gap-3 rounded-2xl border border-[var(--border-button)] bg-[var(--bg-input)] p-4 text-left transition hover:border-[var(--accent-primary)] hover:bg-[var(--bg-component)]"
              key={dataset.name}
              onClick={() => navigate('/datasets')}
              type="button"
            >
              <span className="inline-flex size-10 shrink-0 items-center justify-center rounded-xl bg-[var(--bg-card)] text-[var(--accent-purple)]">
                <FileText className="size-5" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium text-[var(--text)]">
                  {dataset.name}
                </span>
                <span className="mt-1 block text-sm text-[var(--text-muted)]">
                  {dataset.files}
                </span>
              </span>
              <span className="rounded-full bg-[var(--pill-review-bg)] px-3 py-1 text-xs font-medium text-[var(--pill-review-text)]">
                {dataset.status}
              </span>
            </button>
          ))}
        </div>

        <Button
          className="mt-3 min-h-12 rounded-2xl border-dashed border-[var(--border-button)] bg-transparent text-[var(--text-muted)] hover:border-[var(--accent-primary)] hover:bg-[var(--bg-input)] hover:text-[var(--text)]"
          size="auto"
          variant="outline"
        >
          <Upload className="size-4" />
          Upload dataset files
        </Button>
      </div>
    </section>
  );
}
