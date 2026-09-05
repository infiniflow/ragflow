import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import { CheckCircle2, ChevronDown, CircleHelp, CircleOff } from 'lucide-react';
import { type Ref, useEffect, useState } from 'react';
import type { BusinessDocumentQuestion } from '../types';
import { appendVoiceTranscript, VoiceInput } from './voice-input';

const questionStatusPresentation = {
  OPEN: {
    label: 'Требует ответа',
    icon: CircleHelp,
    frameClass: 'bg-accent-primary/5',
    railClass: 'bg-accent-primary',
    iconClass: 'bg-accent-primary/10 text-accent-primary',
    badgeClass:
      'border-accent-primary/20 bg-accent-primary/10 text-accent-primary',
  },
  ANSWERED: {
    label: 'Ответ зафиксирован',
    icon: CheckCircle2,
    frameClass: 'bg-state-success/5',
    railClass: 'bg-state-success',
    iconClass: 'bg-state-success/10 text-state-success',
    badgeClass:
      'border-state-success/20 bg-state-success/10 text-state-success',
  },
  CANCELLED: {
    label: 'Закрыт',
    icon: CircleOff,
    frameClass: 'bg-bg-card/40',
    railClass: 'bg-text-disabled',
    iconClass: 'bg-bg-card text-text-disabled',
    badgeClass: 'border-border-button bg-bg-card text-text-disabled',
  },
} satisfies Record<
  BusinessDocumentQuestion['status'],
  {
    label: string;
    icon: typeof CircleHelp;
    frameClass: string;
    railClass: string;
    iconClass: string;
    badgeClass: string;
  }
>;

interface QuestionItemProps {
  question: BusinessDocumentQuestion;
  canAnswer: boolean;
  pending: boolean;
  expanded: boolean;
  triggerRef: Ref<HTMLButtonElement>;
  onExpandedChange: (expanded: boolean) => void;
  onAnswer: (payload: Record<string, unknown>) => void;
}

export function QuestionItem({
  question,
  canAnswer,
  pending,
  expanded,
  triggerRef,
  onExpandedChange,
  onAnswer,
}: QuestionItemProps) {
  const [selectedOptionId, setSelectedOptionId] = useState('');
  const [customAnswer, setCustomAnswer] = useState('');
  const status = questionStatusPresentation[question.status];
  const StatusIcon = status.icon;
  const isOpen = question.status === 'OPEN';
  const canSubmit =
    canAnswer &&
    isOpen &&
    !pending &&
    Boolean(selectedOptionId || customAnswer.trim());

  useEffect(() => {
    setSelectedOptionId(question.answer?.selected_option_id ?? '');
    setCustomAnswer(question.answer?.custom_answer ?? '');
  }, [question]);

  return (
    <Collapsible open={expanded} onOpenChange={onExpandedChange} asChild>
      <article
        className={cn(
          'relative overflow-hidden border-b border-border-button px-5 py-5 transition-colors duration-200 last:border-b-0',
          status.frameClass,
        )}
        data-testid="business-document-question"
        data-status={question.status}
      >
        <span
          aria-hidden="true"
          className={cn('absolute inset-y-0 start-0 w-0.5', status.railClass)}
        />
        <div className="flex items-start gap-3">
          <span
            className={cn(
              'mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md transition-colors duration-200',
              status.iconClass,
            )}
          >
            <StatusIcon className="size-4" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex items-start gap-2">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <span className="text-xs font-medium uppercase tracking-wide text-text-secondary">
                    Вопрос
                    {question.sequence_number
                      ? ` ${question.sequence_number}`
                      : ''}
                  </span>
                  {question.target_section_id && (
                    <span className="font-mono text-[11px] text-text-disabled">
                      § {question.target_section_id}
                    </span>
                  )}
                  <Badge
                    variant="outline"
                    className={cn(
                      'h-5 px-2 py-0 text-[10px] font-medium leading-none',
                      status.badgeClass,
                    )}
                    data-testid={`question-status-${question.question_id}`}
                  >
                    {status.label}
                  </Badge>
                </div>
                <h3
                  className={cn(
                    'mt-2 whitespace-pre-wrap break-words text-sm font-medium leading-6 text-text-primary',
                    !expanded && 'line-clamp-2',
                  )}
                >
                  {question.text}
                </h3>
              </div>
              <CollapsibleTrigger asChild>
                <button
                  ref={triggerRef}
                  type="button"
                  className="mt-0.5 inline-flex size-8 shrink-0 items-center justify-center rounded-md text-text-secondary transition-colors hover:bg-bg-card hover:text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary"
                  aria-label={`${expanded ? 'Свернуть' : 'Развернуть'} вопрос${
                    question.sequence_number
                      ? ` ${question.sequence_number}`
                      : ''
                  }: ${question.text}`}
                  data-testid={`toggle-question-${question.question_id}`}
                >
                  <ChevronDown
                    className={`size-4 transition-transform duration-200 ${
                      expanded ? 'rotate-180' : ''
                    }`}
                  />
                </button>
              </CollapsibleTrigger>
            </div>

            <CollapsibleContent className="overflow-hidden data-[state=closed]:animate-accordion-up data-[state=open]:animate-accordion-down">
              <RadioGroup
                className="mt-4 gap-2"
                value={selectedOptionId}
                disabled={!canAnswer || !isOpen || pending}
                onValueChange={(value) => {
                  setSelectedOptionId(value);
                  setCustomAnswer('');
                }}
              >
                {question.options.map((option) => (
                  <label
                    key={option.option_id}
                    className="flex cursor-pointer items-start gap-2.5 rounded-md px-2 py-2 text-sm transition-colors hover:bg-bg-card has-[:disabled]:cursor-default"
                  >
                    <RadioGroupItem
                      value={option.option_id}
                      aria-label={option.label}
                      className="mt-0.5"
                    />
                    <span className="min-w-0">
                      <span className="block text-text-primary">
                        {option.label}
                      </span>
                      {option.description && (
                        <span className="mt-0.5 block text-xs leading-5 text-text-secondary">
                          {option.description}
                        </span>
                      )}
                    </span>
                  </label>
                ))}
              </RadioGroup>

              {question.allow_custom_answer && isOpen && (
                <div className="relative mt-3">
                  <Textarea
                    value={customAnswer}
                    aria-label="Свой ответ"
                    placeholder="Свой ответ"
                    className="min-h-20 pe-11"
                    disabled={!canAnswer || pending}
                    onChange={(event) => {
                      setCustomAnswer(event.target.value);
                      if (event.target.value) setSelectedOptionId('');
                    }}
                  />
                  <div className="absolute end-2 top-2">
                    <VoiceInput
                      label="Ответ на вопрос"
                      disabled={!canAnswer || pending}
                      onTranscript={(transcript) => {
                        setCustomAnswer((value) =>
                          appendVoiceTranscript(value, transcript),
                        );
                        setSelectedOptionId('');
                      }}
                      testId={`voice-input-question-${question.question_id}`}
                    />
                  </div>
                </div>
              )}

              {isOpen ? (
                <div className="mt-3 flex items-center justify-between gap-3">
                  {!canAnswer && (
                    <span className="text-xs text-text-disabled">
                      Ответ сейчас недоступен
                    </span>
                  )}
                  <Button
                    size="sm"
                    variant="accent"
                    className="ms-auto"
                    data-testid={`answer-question-${question.question_id}`}
                    disabled={!canSubmit}
                    loading={pending}
                    onClick={() =>
                      onAnswer({
                        question_id: question.question_id,
                        selected_option_id: selectedOptionId || null,
                        custom_answer: customAnswer.trim() || null,
                      })
                    }
                  >
                    Ответить
                  </Button>
                </div>
              ) : null}
            </CollapsibleContent>
          </div>
        </div>
      </article>
    </Collapsible>
  );
}
