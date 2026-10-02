import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Panel } from '@xyflow/react';
import { ArrowDown, ArrowLeft, ArrowRight, ArrowUp } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  CanvasLayoutAlgorithm,
  CanvasLayoutDirection,
  useCanvasEdgeRoute,
} from '../utils/canvas-edge-route';

const directions: Array<{
  id: CanvasLayoutDirection;
  icon: typeof ArrowRight;
  label: string;
}> = [
  { id: 'LR', icon: ArrowRight, label: 'Left to right' },
  { id: 'RL', icon: ArrowLeft, label: 'Right to left' },
  { id: 'TB', icon: ArrowDown, label: 'Top to bottom' },
  { id: 'BT', icon: ArrowUp, label: 'Bottom to top' },
];

export function CanvasLayoutPanel({ onArrange }: { onArrange: () => void }) {
  const { t } = useTranslation();
  const settings = useCanvasEdgeRoute((state) => state.settings);
  const setSettings = useCanvasEdgeRoute((state) => state.setSettings);

  return (
    <Panel
      position="top-right"
      className="nodrag nopan m-3 w-60 space-y-3 rounded-lg border border-border-button bg-bg-base p-3 text-xs text-text-primary shadow-md"
    >
      <label className="block space-y-1">
        <span className="text-text-secondary">{t('flow.layoutAlgorithm')}</span>
        <Select
          value={settings.algorithm}
          onValueChange={(value) => {
            if (value === 'elk' || value === 'dagre') {
              setSettings({ algorithm: value as CanvasLayoutAlgorithm });
            }
          }}
        >
          <SelectTrigger className="h-8 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="elk">ELK</SelectItem>
            <SelectItem value="dagre">Dagre</SelectItem>
          </SelectContent>
        </Select>
      </label>

      <div className="space-y-1">
        <span className="text-text-secondary">{t('flow.layoutDirection')}</span>
        <div className="grid grid-cols-4 gap-1">
          {directions.map((item) => {
            const Icon = item.icon;
            const selected = settings.direction === item.id;
            return (
              <button
                key={item.id}
                type="button"
                aria-label={t(`flow.layoutDirection${item.id}`)}
                title={t(`flow.layoutDirection${item.id}`)}
                className={
                  selected
                    ? 'flex h-8 items-center justify-center rounded-md border border-accent-primary bg-bg-base-hover text-text-primary'
                    : 'flex h-8 items-center justify-center rounded-md border border-border-button text-text-secondary hover:bg-bg-base-hover hover:text-text-primary'
                }
                onClick={() =>
                  setSettings({ direction: item.id as CanvasLayoutDirection })
                }
              >
                <Icon className="h-3.5 w-3.5" />
              </button>
            );
          })}
        </div>
      </div>

      <div className="grid grid-cols-2 gap-2">
        <label className="space-y-1">
          <span className="text-text-secondary">
            {t('flow.layoutNodeSpacing')}
          </span>
          <input
            type="number"
            min={16}
            max={400}
            value={settings.nodeSpacing}
            className="h-8 w-full rounded-md border border-border-button bg-bg-input px-2 text-xs"
            onChange={(event) =>
              setSettings({ nodeSpacing: Number(event.target.value) })
            }
          />
        </label>
        <label className="space-y-1">
          <span className="text-text-secondary">
            {t('flow.layoutRankSpacing')}
          </span>
          <input
            type="number"
            min={16}
            max={400}
            value={settings.rankSpacing}
            className="h-8 w-full rounded-md border border-border-button bg-bg-input px-2 text-xs"
            onChange={(event) =>
              setSettings({ rankSpacing: Number(event.target.value) })
            }
          />
        </label>
      </div>

      <label className="block space-y-1">
        <span className="text-text-secondary">{t('flow.edgeRoute')}</span>
        <Select
          value={settings.route}
          onValueChange={(value) => {
            if (value === 'bezier' || value === 'orthogonal') {
              setSettings({ route: value });
            }
          }}
        >
          <SelectTrigger className="h-8 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="bezier">{t('flow.edgeRouteBezier')}</SelectItem>
            <SelectItem value="orthogonal">
              {t('flow.edgeRouteOrthogonal')}
            </SelectItem>
          </SelectContent>
        </Select>
      </label>

      <button
        type="button"
        className="h-8 w-full rounded-md bg-accent-primary text-xs font-medium text-white hover:opacity-90"
        onClick={onArrange}
      >
        {t('flow.autoArrange')}
      </button>
    </Panel>
  );
}
