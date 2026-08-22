import { MoreButton } from '@/components/more-button';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { TruncatedText } from '@/components/truncated-text';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';
import { formatDate } from '@/utils/date';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import { formatKindLabel } from '@/utils/compilation-template-util';
import { CompilationTemplateDropdown } from './compilation-template-dropdown';
import { FlowType, FlowTypeConfig } from './constant';

type CompilationTemplateCardProps = {
  data: ICompilationTemplateGroup;
  onClick?: () => void;
  onDelete: (id: string) => void;
};

const CompilerConfig = FlowTypeConfig[FlowType.Compiler];
const CompilerIcon = CompilerConfig.icon;

export function CompilationTemplateCard({
  data,
  onClick,
  onDelete,
}: CompilationTemplateCardProps) {
  const { t } = useTranslation();
  const kinds = useMemo(
    () => Array.from(new Set((data.templates ?? []).map((item) => item.kind))),
    [data.templates],
  );

  return (
    <Card className="group cursor-pointer h-full" onClick={onClick}>
      <CardContent className="py-4 px-2.5 flex gap-3">
        <RAGFlowAvatar
          avatar={data.avatar}
          name={data.name}
          className="w-8 h-8 shrink-0"
        />

        <div className="flex-1 min-w-0 flex flex-col gap-1">
          <section className="flex items-center justify-between gap-2">
            <TruncatedText
              as="h3"
              className="flex-1 min-w-0 truncate"
              tooltip={data.name}
            >
              {data.name}
            </TruncatedText>

            <Button variant="ghost" size="sm">
              <CompilerIcon style={{ color: CompilerConfig.color }} />
            </Button>

            <CompilationTemplateDropdown data={data} onDelete={onDelete}>
              <MoreButton />
            </CompilationTemplateDropdown>
          </section>

          <TruncatedText
            as="p"
            className="text-sm text-text-secondary line-clamp-1"
            tooltip={data.description}
          >
            {data.description}
          </TruncatedText>

          <div className="flex flex-wrap gap-2 mt-2">
            {kinds.map((kind) => (
              <Badge key={kind} variant="secondary">
                {formatKindLabel(t, kind)}
              </Badge>
            ))}
          </div>

          <div className="flex items-center gap-2 mt-1 min-w-0 text-sm text-text-secondary">
            <span className="whitespace-nowrap">{t('flow.lastSavedAt')}:</span>
            <p className="truncate">{formatDate(data.update_time)}</p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
