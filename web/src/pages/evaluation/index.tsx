import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
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
import { Textarea } from '@/components/ui/textarea';
import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchChatList } from '@/hooks/use-chat-request';
import {
  useAddEvaluationCase,
  useDeleteEvaluationCase,
  useFetchEvaluationCases,
  useFetchEvaluationDataset,
  useFetchEvaluationRecommendations,
  useFetchEvaluationRunResults,
  useFetchEvaluationRuns,
  useImportEvaluationCases,
  useStartEvaluationRun,
} from '@/hooks/use-evaluation-request';
import { Routes } from '@/routes';
import { formatPureDate } from '@/utils/date';
import { ArrowLeft, Play, Plus, Upload } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useParams } from 'react-router';

export default function EvaluationDetailPage() {
  const { id: datasetId } = useParams();
  const { t } = useTranslation();
  const { data: dataset } = useFetchEvaluationDataset(datasetId);
  const { data: casesData, refetch: refetchCases } =
    useFetchEvaluationCases(datasetId);
  const { data: runsData } = useFetchEvaluationRuns(datasetId);
  const [selectedRunId, setSelectedRunId] = useState<string>();
  const runs = runsData?.runs ?? [];
  const selectedRun = runs.find((r) => r.id === selectedRunId);
  const { data: resultsData } = useFetchEvaluationRunResults(
    selectedRunId,
    selectedRun?.status,
  );
  const { data: recommendationsData } =
    useFetchEvaluationRecommendations(selectedRunId, selectedRun?.status);

  const { visible: caseModalVisible, showModal: showCaseModal, hideModal: hideCaseModal } =
    useSetModalState();
  const { visible: runModalVisible, showModal: showRunModal, hideModal: hideRunModal } =
    useSetModalState();

  const [question, setQuestion] = useState('');
  const [referenceAnswer, setReferenceAnswer] = useState('');
  const [chunkIds, setChunkIds] = useState('');
  const [dialogId, setDialogId] = useState('');

  const { mutateAsync: addCase, isPending: addingCase } =
    useAddEvaluationCase(datasetId!);
  const { mutateAsync: deleteCase } = useDeleteEvaluationCase(datasetId!);
  const { mutateAsync: importCases } = useImportEvaluationCases(datasetId!);
  const { mutateAsync: startRun, isPending: startingRun } =
    useStartEvaluationRun(datasetId!);
  const { data: chatListData } = useFetchChatList();

  const cases = casesData?.cases ?? [];
  const chats = chatListData?.chats ?? [];

  const summaryMetrics = useMemo(() => {
    const summary = resultsData?.run?.metrics_summary;
    if (!summary) return [];
    return Object.entries(summary).filter(([k]) => k.startsWith('avg_'));
  }, [resultsData]);

  const handleAddCase = async () => {
    if (!question.trim()) return;
    await addCase({
      question: question.trim(),
      reference_answer: referenceAnswer.trim() || undefined,
      relevant_chunk_ids: chunkIds
        ? chunkIds.split(',').map((s) => s.trim()).filter(Boolean)
        : undefined,
    });
    setQuestion('');
    setReferenceAnswer('');
    setChunkIds('');
    hideCaseModal();
    refetchCases();
  };

  const handleImportCsv = async (file: File) => {
    const text = await file.text();
    const lines = text.split(/\r?\n/).filter((l) => l.trim());
    const parsed = lines.slice(1).map((line) => {
      const [q, ref, chunks] = line.split(',').map((s) => s.trim());
      return {
        question: q,
        reference_answer: ref || undefined,
        relevant_chunk_ids: chunks
          ? chunks.split('|').map((c) => c.trim()).filter(Boolean)
          : undefined,
      };
    }).filter((row) => row.question);
    await importCases(parsed);
    refetchCases();
  };

  const handleStartRun = async () => {
    if (!dialogId) return;
    const res = await startRun({ dialog_id: dialogId });
    setSelectedRunId(res.id);
    hideRunModal();
  };

  return (
    <div className="size-full flex flex-col px-5 py-8 gap-6 overflow-auto">
      <header className="flex items-center gap-4">
        <Button variant="ghost" size="icon" asChild>
          <Link to={Routes.Evaluations} aria-label={t('common.back')}>
            <ArrowLeft className="size-4" />
          </Link>
        </Button>
        <div className="min-w-0 flex-1">
          <h1 className="text-2xl font-semibold truncate">{dataset?.name}</h1>
          {dataset?.description && (
            <p className="text-text-secondary text-sm mt-1">{dataset.description}</p>
          )}
        </div>
        <Button onClick={showRunModal} disabled={!cases.length}>
          <Play className="size-4 mr-2" />
          {t('evaluation.runEvaluation')}
        </Button>
      </header>

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle>{t('evaluation.testCases')}</CardTitle>
              <CardDescription>{t('evaluation.testCasesDescription')}</CardDescription>
            </div>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" asChild>
                <label className="cursor-pointer">
                  <Upload className="size-4 mr-1" />
                  {t('evaluation.importCsv')}
                  <input
                    type="file"
                    accept=".csv"
                    className="hidden"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) handleImportCsv(file);
                      e.target.value = '';
                    }}
                  />
                </label>
              </Button>
              <Button size="sm" onClick={showCaseModal}>
                <Plus className="size-4 mr-1" />
                {t('evaluation.addCase')}
              </Button>
            </div>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('evaluation.question')}</TableHead>
                  <TableHead>{t('evaluation.referenceAnswer')}</TableHead>
                  <TableHead className="w-20" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {cases.map((c) => (
                  <TableRow key={c.id}>
                    <TableCell className="max-w-xs truncate">{c.question}</TableCell>
                    <TableCell className="max-w-xs truncate text-text-secondary">
                      {c.reference_answer || '—'}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => deleteCase(c.id)}
                      >
                        {t('common.delete')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
                {!cases.length && (
                  <TableRow>
                    <TableCell colSpan={3} className="text-center text-text-secondary">
                      {t('evaluation.noCases')}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('evaluation.runs')}</CardTitle>
            <CardDescription>{t('evaluation.runsDescription')}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            {runs.map((run) => (
              <button
                key={run.id}
                type="button"
                className={`text-left rounded-lg border px-4 py-3 transition-colors ${
                  selectedRunId === run.id
                    ? 'border-text-primary bg-bg-card'
                    : 'border-border-button hover:border-text-secondary'
                }`}
                onClick={() => setSelectedRunId(run.id)}
              >
                <div className="font-medium">{run.name}</div>
                <div className="text-sm text-text-secondary mt-1">
                  {run.status} · {formatPureDate(run.create_time)}
                </div>
              </button>
            ))}
            {!runs.length && (
              <p className="text-text-secondary text-sm">{t('evaluation.noRuns')}</p>
            )}
          </CardContent>
        </Card>
      </div>

      {selectedRunId && resultsData && (
        <Card>
          <CardHeader>
            <CardTitle>{t('evaluation.results')}</CardTitle>
            {summaryMetrics.length > 0 && (
              <CardDescription className="flex flex-wrap gap-4 mt-2">
                {summaryMetrics.map(([key, value]) => (
                  <span key={key}>
                    {key.replace('avg_', '')}: {(value as number).toFixed(3)}
                  </span>
                ))}
              </CardDescription>
            )}
          </CardHeader>
          <CardContent className="flex flex-col gap-6">
            {recommendationsData?.recommendations?.length ? (
              <div className="rounded-lg border border-border-button p-4 bg-bg-card">
                <h3 className="font-medium mb-2">{t('evaluation.recommendations')}</h3>
                <ul className="list-disc pl-5 space-y-2 text-sm">
                  {recommendationsData.recommendations.map((rec, i) => (
                    <li key={i}>
                      <strong>{rec.issue}</strong> ({rec.severity}): {rec.description}
                      <ul className="list-disc pl-5 mt-1 text-text-secondary">
                        {rec.suggestions.map((s, j) => (
                          <li key={j}>{s}</li>
                        ))}
                      </ul>
                    </li>
                  ))}
                </ul>
              </div>
            ) : null}

            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('evaluation.metrics')}</TableHead>
                  <TableHead>{t('evaluation.answer')}</TableHead>
                  <TableHead>{t('evaluation.time')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {resultsData.results?.map((result) => (
                  <TableRow key={result.id}>
                    <TableCell className="text-xs">
                      {Object.entries(result.metrics || {})
                        .map(([k, v]) => `${k}: ${Number(v).toFixed(3)}`)
                        .join(', ') || '—'}
                    </TableCell>
                    <TableCell className="max-w-md truncate text-sm">
                      {result.generated_answer}
                    </TableCell>
                    <TableCell>{result.execution_time?.toFixed(2)}s</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      <Dialog open={caseModalVisible} onOpenChange={(o) => !o && hideCaseModal()}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('evaluation.addCase')}</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <Label>{t('evaluation.question')}</Label>
              <Textarea value={question} onChange={(e) => setQuestion(e.target.value)} rows={3} />
            </div>
            <div className="flex flex-col gap-2">
              <Label>{t('evaluation.referenceAnswer')}</Label>
              <Textarea value={referenceAnswer} onChange={(e) => setReferenceAnswer(e.target.value)} rows={3} />
            </div>
            <div className="flex flex-col gap-2">
              <Label>{t('evaluation.relevantChunkIds')}</Label>
              <Input
                value={chunkIds}
                onChange={(e) => setChunkIds(e.target.value)}
                placeholder={t('evaluation.chunkIdsPlaceholder')}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={hideCaseModal}>{t('common.cancel')}</Button>
            <Button onClick={handleAddCase} disabled={!question.trim() || addingCase}>
              {t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={runModalVisible} onOpenChange={(o) => !o && hideRunModal()}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('evaluation.runEvaluation')}</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <Label>{t('evaluation.selectChat')}</Label>
            <Select value={dialogId} onValueChange={setDialogId}>
              <SelectTrigger>
                <SelectValue placeholder={t('evaluation.selectChatPlaceholder')} />
              </SelectTrigger>
              <SelectContent>
                {chats.map((chat) => (
                  <SelectItem key={chat.id} value={chat.id}>
                    {chat.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={hideRunModal}>{t('common.cancel')}</Button>
            <Button onClick={handleStartRun} disabled={!dialogId || startingRun}>
              {t('evaluation.startRun')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
