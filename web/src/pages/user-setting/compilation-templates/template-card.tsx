import { MoreButton } from '@/components/more-button';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent } from '@/components/ui/card';
import { CompilationTemplateScope } from '@/constants/compilation';
import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';
import { Database, FileText, LucideIcon } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { TemplateDropdown } from './template-dropdown';

type TemplateCardProps = {
  data: ICompilationTemplateGroup;
  onClick?: () => void;
  onDelete: (id: string) => void;
};

const ScopeIconMap: Record<string, LucideIcon> = {
  [CompilationTemplateScope.File]: FileText,
  [CompilationTemplateScope.Dataset]: Database,
};

function ScopeIcon({ scope }: { scope?: string }) {
  const Icon = scope ? ScopeIconMap[scope] : null;

  return Icon ? <Icon className="size-4 text-text-secondary shrink-0" /> : null;
}

export function TemplateCard({ data, onClick, onDelete }: TemplateCardProps) {
  const { t } = useTranslation();

  const kinds = useMemo(
    () => Array.from(new Set((data.templates ?? []).map((item) => item.kind))),
    [data.templates],
  );

  return (
    <Card className="group cursor-pointer h-full" onClick={onClick}>
      <CardContent className="p-4 flex gap-3">
        <RAGFlowAvatar
          avatar={data.avatar}
          name={data.name}
          className="w-8 h-8 shrink-0"
        />

        <div className="flex-1 min-w-0 flex flex-col gap-1">
          <section className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-2 min-w-0 flex-1">
              <h3 className="text-base font-normal truncate text-text-primary">
                {data.name}
              </h3>
              <ScopeIcon scope={data.scope} />
            </div>

            <TemplateDropdown data={data} onDelete={onDelete}>
              <MoreButton />
            </TemplateDropdown>
          </section>

          <p className="text-sm text-text-secondary line-clamp-1">
            {data.description}
          </p>

          <div className="flex flex-wrap gap-2 mt-2">
            {kinds.map((kind) => (
              <Badge key={kind} variant="secondary">
                {t(`knowledgeCompilation.kind.${kind}`, kind)}
              </Badge>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
