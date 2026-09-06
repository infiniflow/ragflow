import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import {
  Check,
  CheckCircle2,
  CircleMinus,
  Lightbulb,
  X,
  XCircle,
} from 'lucide-react';
import type { BusinessDocumentProposal } from '../types';

interface ProposalItemProps {
  proposal: BusinessDocumentProposal;
  proposalDecisionsOpen: boolean;
  canDecide: boolean;
  pending: boolean;
  onDecide: (payload: Record<string, unknown>) => void;
}

const proposalStatusPresentation = {
  PENDING: {
    label: 'Ожидает решения',
    icon: Lightbulb,
    frameClass: 'bg-state-warning/5',
    railClass: 'bg-state-warning',
    iconClass: 'bg-state-warning/10 text-state-warning',
    badgeClass:
      'border-state-warning/20 bg-state-warning/10 text-state-warning',
  },
  ACCEPTED: {
    label: 'Принято',
    icon: CheckCircle2,
    frameClass: 'bg-state-success/5',
    railClass: 'bg-state-success',
    iconClass: 'bg-state-success/10 text-state-success',
    badgeClass:
      'border-state-success/20 bg-state-success/10 text-state-success',
  },
  REJECTED: {
    label: 'Отклонено',
    icon: XCircle,
    frameClass: 'bg-state-error/5',
    railClass: 'bg-state-error',
    iconClass: 'bg-state-error/10 text-state-error',
    badgeClass: 'border-state-error/20 bg-state-error/10 text-state-error',
  },
} satisfies Record<
  BusinessDocumentProposal['decision'],
  {
    label: string;
    icon: typeof Lightbulb;
    frameClass: string;
    railClass: string;
    iconClass: string;
    badgeClass: string;
  }
>;

const closedPendingProposalPresentation = {
  label: 'Не принято',
  icon: CircleMinus,
  frameClass: 'bg-bg-card/40',
  railClass: 'bg-text-disabled',
  iconClass: 'bg-bg-card text-text-disabled',
  badgeClass: 'border-border-button bg-bg-card text-text-disabled',
};

export function ProposalItem({
  proposal,
  proposalDecisionsOpen,
  canDecide,
  pending,
  onDecide,
}: ProposalItemProps) {
  const isPending = proposal.decision === 'PENDING';
  const isClosedWithoutDecision = isPending && !proposalDecisionsOpen;
  const status = isClosedWithoutDecision
    ? closedPendingProposalPresentation
    : proposalStatusPresentation[proposal.decision];
  const StatusIcon = status.icon;

  return (
    <article
      className={cn(
        'relative overflow-hidden border-b border-border-button px-5 py-5 transition-colors duration-200 last:border-b-0',
        status.frameClass,
      )}
      data-testid="business-document-proposal"
      data-status={proposal.decision}
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
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-xs font-medium uppercase tracking-wide text-text-secondary">
              Предложение
            </span>
            <Badge
              variant="outline"
              className={cn(
                'h-5 px-2 py-0 text-[10px] font-medium leading-none',
                status.badgeClass,
              )}
              data-testid={`proposal-status-${proposal.proposal_id}`}
            >
              {status.label}
            </Badge>
          </div>
          <p className="mt-2 text-sm leading-6 text-text-primary">
            {proposal.text}
          </p>
          {proposal.rationale && (
            <p className="mt-2 text-xs leading-5 text-text-secondary">
              {proposal.rationale}
            </p>
          )}
          {isClosedWithoutDecision && (
            <p
              className="mt-3 text-xs leading-5 text-text-secondary"
              data-testid={`closed-proposal-${proposal.proposal_id}`}
            >
              Решение не было принято до завершения этапа внесения изменений.
            </p>
          )}
          {isPending && proposalDecisionsOpen && (
            <div className="mt-4 flex gap-2">
              <Button
                size="sm"
                variant="danger"
                disabled={!canDecide || pending}
                data-testid={`reject-proposal-${proposal.proposal_id}`}
                onClick={() =>
                  onDecide({
                    proposal_id: proposal.proposal_id,
                    decision: 'REJECTED',
                  })
                }
              >
                <X className="size-3.5" />
                Отклонить
              </Button>
              <Button
                size="sm"
                variant="accent"
                disabled={!canDecide || pending}
                loading={pending}
                data-testid={`accept-proposal-${proposal.proposal_id}`}
                onClick={() =>
                  onDecide({
                    proposal_id: proposal.proposal_id,
                    decision: 'ACCEPTED',
                  })
                }
              >
                <Check className="size-3.5" />
                Принять
              </Button>
            </div>
          )}
        </div>
      </div>
    </article>
  );
}
