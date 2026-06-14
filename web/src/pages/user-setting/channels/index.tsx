import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  createChannel,
  deleteChannel,
  getChannels,
  IChannel,
  IChannelPayload,
  updateChannel,
} from '@/services/channel';
import { MoreHorizontal, Pencil, Plus, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

const CHANNEL_TYPES = ['slack', 'feishu'];

const EMPTY_FORM: IChannelPayload = {
  name: '',
  channel: 'slack',
  dialog_id: '',
  config: { credential: {} },
  status: 'enabled',
};

function ChannelFormDialog({
  open,
  initial,
  onClose,
  onSaved,
}: {
  open: boolean;
  initial: IChannelPayload & { id?: string };
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const [form, setForm] = useState<IChannelPayload & { id?: string }>(initial);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setForm(initial);
  }, [initial]);

  const set = (field: keyof IChannelPayload, value: unknown) =>
    setForm((prev) => ({ ...prev, [field]: value }));

  const setCredential = (key: string, value: string) =>
    setForm((prev) => ({
      ...prev,
      config: {
        ...prev.config,
        credential: {
          ...((prev.config.credential as Record<string, string>) ?? {}),
          [key]: value,
        },
      },
    }));

  async function handleSave() {
    setSaving(true);
    try {
      if (form.id) {
        await updateChannel(form.id, form);
      } else {
        await createChannel(form);
      }
      onSaved();
      onClose();
    } finally {
      setSaving(false);
    }
  }

  const cred = (form.config.credential ?? {}) as Record<string, string>;

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {form.id ? t('setting.editChannel') : t('setting.addChannel')}
          </DialogTitle>
        </DialogHeader>

        <div className="grid gap-4 py-2">
          <div className="grid gap-1.5">
            <Label>{t('setting.channelName')}</Label>
            <Input
              value={form.name}
              onChange={(e) => set('name', e.target.value)}
              placeholder="My Slack Bot"
            />
          </div>

          <div className="grid gap-1.5">
            <Label>{t('setting.channelType')}</Label>
            <Select
              value={form.channel}
              onValueChange={(v) => set('channel', v)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CHANNEL_TYPES.map((ct) => (
                  <SelectItem key={ct} value={ct}>
                    {ct}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="grid gap-1.5">
            <Label>{t('setting.assistantId')}</Label>
            <Input
              value={form.dialog_id}
              onChange={(e) => set('dialog_id', e.target.value)}
              placeholder="dialog ID"
            />
          </div>

          {form.channel === 'slack' && (
            <>
              <div className="grid gap-1.5">
                <Label>Bot Token</Label>
                <Input
                  type="password"
                  value={cred.bot_token ?? ''}
                  onChange={(e) => setCredential('bot_token', e.target.value)}
                  placeholder="xoxb-..."
                />
              </div>
              <div className="grid gap-1.5">
                <Label>App Token</Label>
                <Input
                  type="password"
                  value={cred.app_token ?? ''}
                  onChange={(e) => setCredential('app_token', e.target.value)}
                  placeholder="xapp-..."
                />
              </div>
            </>
          )}

          {form.channel === 'feishu' && (
            <>
              <div className="grid gap-1.5">
                <Label>App ID</Label>
                <Input
                  value={cred.app_id ?? ''}
                  onChange={(e) => setCredential('app_id', e.target.value)}
                />
              </div>
              <div className="grid gap-1.5">
                <Label>App Secret</Label>
                <Input
                  type="password"
                  value={cred.app_secret ?? ''}
                  onChange={(e) => setCredential('app_secret', e.target.value)}
                />
              </div>
              <div className="grid gap-1.5">
                <Label>Encrypt Key (optional)</Label>
                <Input
                  value={cred.encrypt_key ?? ''}
                  onChange={(e) => setCredential('encrypt_key', e.target.value)}
                />
              </div>
            </>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={handleSave}
            disabled={saving || !form.name || !form.dialog_id}
          >
            {saving ? t('common.saving') : t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default function ChannelsPage() {
  const { t } = useTranslation();
  const [channels, setChannels] = useState<IChannel[]>([]);
  const [loading, setLoading] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<
    IChannelPayload & { id?: string }
  >(EMPTY_FORM);

  const fetchChannels = useCallback(async () => {
    setLoading(true);
    try {
      setChannels(await getChannels());
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchChannels();
  }, [fetchChannels]);

  function openAdd() {
    setEditTarget({ ...EMPTY_FORM, config: { credential: {} } });
    setDialogOpen(true);
  }

  function openEdit(ch: IChannel) {
    setEditTarget({
      id: ch.id,
      name: ch.name,
      channel: ch.channel,
      dialog_id: ch.dialog_id,
      config: ch.config,
      status: ch.status,
    });
    setDialogOpen(true);
  }

  async function handleDelete(id: string) {
    await deleteChannel(id);
    fetchChannels();
  }

  async function toggleStatus(ch: IChannel) {
    await updateChannel(ch.id, {
      status: ch.status === 'enabled' ? 'disabled' : 'enabled',
    });
    fetchChannels();
  }

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">{t('setting.chatChannels')}</h2>
        <Button onClick={openAdd} size="sm" className="gap-1.5">
          <Plus className="size-4" />
          {t('setting.addChannel')}
        </Button>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('setting.channelName')}</TableHead>
            <TableHead>{t('setting.channelType')}</TableHead>
            <TableHead>{t('setting.status')}</TableHead>
            <TableHead className="w-10" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading && (
            <TableRow>
              <TableCell
                colSpan={4}
                className="text-center text-muted-foreground py-8"
              >
                {t('common.loading')}
              </TableCell>
            </TableRow>
          )}
          {!loading && channels.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={4}
                className="text-center text-muted-foreground py-8"
              >
                {t('setting.noChannels')}
              </TableCell>
            </TableRow>
          )}
          {channels.map((ch) => (
            <TableRow key={ch.id}>
              <TableCell className="font-medium">{ch.name}</TableCell>
              <TableCell>{ch.channel}</TableCell>
              <TableCell>
                <span
                  className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
                    ch.status === 'enabled'
                      ? 'bg-green-100 text-green-700'
                      : 'bg-zinc-100 text-zinc-500'
                  }`}
                >
                  {ch.status}
                </span>
              </TableCell>
              <TableCell>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon" className="size-8">
                      <MoreHorizontal className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onClick={() => openEdit(ch)}>
                      <Pencil className="mr-2 size-4" />
                      {t('common.edit')}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => toggleStatus(ch)}>
                      {ch.status === 'enabled'
                        ? t('common.disable')
                        : t('common.enable')}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      className="text-destructive"
                      onClick={() => handleDelete(ch.id)}
                    >
                      <Trash2 className="mr-2 size-4" />
                      {t('common.delete')}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <ChannelFormDialog
        open={dialogOpen}
        initial={editTarget}
        onClose={() => setDialogOpen(false)}
        onSaved={fetchChannels}
      />
    </div>
  );
}
