import LLMLabel from '@/components/llm-select/llm-label';
import { ModelTypeMap } from '@/components/model-tree-select';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  useCompilationTemplateGroupOptions,
  useCompilationTemplateGroupValidIds,
} from '@/hooks/use-compilation-template-group-request';
import { useModelValidIds } from '@/hooks/use-llm-request';
import { cn } from '@/lib/utils';
import { TriangleAlert } from 'lucide-react';
import { PropsWithChildren } from 'react';
import { useTranslation } from 'react-i18next';
import { useOwnerTenantId } from '../../context';

export function CardWithForm() {
  return (
    <Card className="w-[350px]">
      <CardHeader>
        <CardTitle>Create project</CardTitle>
        <CardDescription>Deploy your new project in one-click.</CardDescription>
      </CardHeader>
      <CardContent>
        <form>
          <div className="grid w-full items-center gap-4">
            <div className="flex flex-col space-y-1.5">
              <Label htmlFor="name">Name</Label>
              <Input id="name" placeholder="Name of your project" />
            </div>
            <div className="flex flex-col space-y-1.5">
              <Label htmlFor="framework">Framework</Label>
              <Select>
                <SelectTrigger id="framework">
                  <SelectValue placeholder="Select" />
                </SelectTrigger>
                <SelectContent position="popper">
                  <SelectItem value="next">Next.js</SelectItem>
                  <SelectItem value="sveltekit">SvelteKit</SelectItem>
                  <SelectItem value="astro">Astro</SelectItem>
                  <SelectItem value="nuxt">Nuxt.js</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        </form>
      </CardContent>
      <CardFooter className="flex justify-between">
        <Button variant="outline">Cancel</Button>
        <Button>Deploy</Button>
      </CardFooter>
    </Card>
  );
}

type LabelCardProps = {
  className?: string;
} & PropsWithChildren &
  React.HTMLAttributes<HTMLElement>;

export function LabelCard({ children, className, ...props }: LabelCardProps) {
  return (
    <div
      className={cn(
        'bg-bg-card rounded-sm p-1 text-text-secondary text-xs',
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}

export function LLMLabelCard({ llmId }: { llmId?: string }) {
  const ownerTenantId = useOwnerTenantId();
  const { t } = useTranslation();
  // Validity is checked against the current user's own models — runs
  // resolve llm_id against the runner's tenant, so a model only the
  // canvas owner has added is unusable. Gated on isFetched so a slow
  // list never flashes a false error state. The display name still
  // resolves through the owner's list.
  const { validIds, isFetched } = useModelValidIds(ModelTypeMap.llm_id);

  const isUnavailable = !!llmId && isFetched && !validIds.has(llmId);
  // An empty model keeps the historical red state; a loading list shows nothing.
  const isInvalid = llmId ? isUnavailable : true;

  return (
    <LabelCard
      className={isInvalid ? 'bg-state-error-5 border-state-error border' : ''}
      title={isUnavailable ? t('common.modelUnavailable') : undefined}
    >
      <span className="flex items-center gap-1.5">
        {isUnavailable && (
          <TriangleAlert className="size-4 shrink-0 text-state-error" />
        )}
        <LLMLabel value={llmId} ownerTenantId={ownerTenantId}></LLMLabel>
      </span>
    </LabelCard>
  );
}

export function CompilationTemplateLabelCard({
  groupId,
}: {
  groupId?: string;
}) {
  const { t } = useTranslation();
  const { options } = useCompilationTemplateGroupOptions();
  // Validity is checked against the current user's own groups — runs resolve
  // the group under the runner's tenant, so a group only the canvas owner can
  // see silently no-ops for anyone else. Gated on isFetched so a slow list
  // never flashes a false error state.
  const { validIds, isFetched } = useCompilationTemplateGroupValidIds();

  const isUnavailable = !!groupId && isFetched && !validIds.has(groupId);
  const groupName =
    options.find((option) => option.value === groupId)?.label ?? groupId;

  return (
    <LabelCard
      className={cn(
        'text-text-primary flex justify-between flex-col gap-1',
        isUnavailable && 'bg-state-error-5 border-state-error border',
      )}
      title={
        isUnavailable
          ? t('knowledgeConfiguration.compilationTemplateUnavailable')
          : undefined
      }
    >
      <span className="text-text-secondary">
        {t('knowledgeConfiguration.compilationTemplate')}
      </span>
      <span className="flex items-center gap-1.5">
        {isUnavailable && (
          <TriangleAlert className="size-4 shrink-0 text-state-error" />
        )}
        <span className="truncate">{groupName}</span>
      </span>
    </LabelCard>
  );
}
