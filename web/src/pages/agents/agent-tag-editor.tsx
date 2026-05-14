import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import {
  useFetchAgentTags,
  useUpdateAgentTags,
} from '@/hooks/use-agent-request';
import { IFlow } from '@/interfaces/database/agent';
import { X } from 'lucide-react';
import { KeyboardEvent, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface IProps {
  agent: IFlow;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const splitTags = (raw?: string) =>
  (raw || '')
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean);

export function AgentTagEditor({ agent, open, onOpenChange }: IProps) {
  const { t } = useTranslation();
  const { loading, updateAgentTags } = useUpdateAgentTags();
  const { data: allTags } = useFetchAgentTags();
  const initial = useMemo(() => splitTags(agent.tags), [agent.tags]);
  const [tags, setTags] = useState<string[]>(initial);
  const [draft, setDraft] = useState('');

  useEffect(() => {
    if (open) {
      setTags(initial);
      setDraft('');
    }
  }, [open, initial]);

  const suggestions = useMemo(() => {
    const taken = new Set(tags.map((t) => t.toLowerCase()));
    const needle = draft.trim().toLowerCase();
    return (allTags ?? [])
      .map((entry) => entry.tag)
      .filter((tag) => !taken.has(tag.toLowerCase()))
      .filter((tag) => !needle || tag.toLowerCase().startsWith(needle))
      .slice(0, 20);
  }, [allTags, tags, draft]);

  const addTag = (tag: string) => {
    const next = tag.trim();
    if (!next) return;
    if (!tags.some((existing) => existing.toLowerCase() === next.toLowerCase())) {
      setTags([...tags, next]);
    }
    setDraft('');
  };

  const commitDraft = () => addTag(draft);

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      commitDraft();
    } else if (e.key === 'Backspace' && !draft && tags.length > 0) {
      setTags(tags.slice(0, -1));
    }
  };

  const removeTag = (tag: string) =>
    setTags(tags.filter((existing) => existing !== tag));

  const handleSave = async () => {
    const pending = draft.trim();
    const alreadyPresent = tags.some(
      (existing) => existing.toLowerCase() === pending.toLowerCase(),
    );
    const finalTags = pending && !alreadyPresent ? [...tags, pending] : tags;
    setTags(finalTags);
    setDraft('');
    const success = await updateAgentTags({
      agentId: agent.id,
      tags: finalTags,
    });
    if (success) {
      onOpenChange(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent onClick={(e) => e.stopPropagation()}>
        <DialogHeader>
          <DialogTitle>{t('flow.editTags')}</DialogTitle>
          <DialogDescription>
            {t('flow.editTagsDescription')}
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-wrap gap-1 min-h-8">
          {tags.map((tag) => (
            <Badge
              key={tag}
              variant="secondary"
              className="text-xs font-normal gap-1"
            >
              {tag}
              <button
                type="button"
                onClick={() => removeTag(tag)}
                aria-label={t('flow.removeTagAriaLabel', { tag })}
                className="hover:text-state-error"
              >
                <X className="size-3" />
              </button>
            </Badge>
          ))}
        </div>

        <Input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={handleKeyDown}
          onBlur={commitDraft}
          placeholder={t('flow.tagsPlaceholder')}
        />

        {suggestions.length > 0 && (
          <div className="space-y-1">
            <div className="text-xs text-text-secondary">
              {t('flow.tagSuggestionsLabel')}
            </div>
            <div className="flex flex-wrap gap-1">
              {suggestions.map((tag) => (
                <button
                  key={tag}
                  type="button"
                  onClick={() => addTag(tag)}
                  className="text-xs"
                >
                  <Badge
                    variant="outline"
                    className="text-xs font-normal cursor-pointer hover:bg-accent"
                  >
                    + {tag}
                  </Badge>
                </button>
              ))}
            </div>
          </div>
        )}

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={loading}
          >
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSave} disabled={loading}>
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
