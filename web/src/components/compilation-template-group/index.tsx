import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { CompilationTemplateKind } from '@/constants/compilation';
import {
  FormEvent,
  ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { BlueprintSection } from './components/blueprint-section';
import { CompilationTemplateEditorSidebar } from './components/editor-sidebar';
import { JsonPreview } from './components/json-preview';
import { TemplateConfiguration } from './components/template-configuration';
import { CompilationTemplateEditorView } from './constant';
import { CompilationTemplateGroupSaveResult } from './hooks/use-compilation-template-group-submit';
import { useEditNextCompilationTemplateGroup } from './hooks/use-edit-next-compilation-template-group';
import {
  draftFromSections,
  OntologySection,
  validateDraft,
} from './ontology/model';

const SelectedTemplateIndex = 0;

/**
 * Form state the surrounding page needs to render its own header: the
 * compiled-output panels ask about the model the form is declaring, so they
 * cannot live inside the editor without leaking into the create dialog.
 */
export type CompilationTemplateGroupEditorState = {
  isOntology: boolean;
  templateId?: string;
  /**
   * Canvas history. The header owns the buttons because it is where the design
   * puts them, but the stack belongs to the form, so the editor hands both the
   * state and the two callbacks up.
   */
  canUndo: boolean;
  canRedo: boolean;
  onUndo: () => void;
  onRedo: () => void;
  /** Whether the form differs from what is stored, for the save badge. */
  isDirty: boolean;
};

type CompilationTemplateGroupEditorProps = {
  id?: string;
  onSuccess: (result: CompilationTemplateGroupSaveResult) => void;
  onCancel: () => void;
  cancelText: string;
  renderHeader?: (state: CompilationTemplateGroupEditorState) => ReactNode;
  sidebarCollapsed?: boolean;
  onSidebarCollapse?: () => void;
};

// Shared body of the edit-next route page and the create dialog: secondary
// navigation, configuration/JSON content, and the cancel/save footer. The
// parent must give it a bounded-height flex column to fill.
export function CompilationTemplateGroupEditor({
  id,
  onSuccess,
  onCancel,
  cancelText,
  renderHeader,
  sidebarCollapsed = false,
  onSidebarCollapse,
}: CompilationTemplateGroupEditorProps) {
  const { t } = useTranslation();
  const { form, kindOptions, builtins, onSubmit, isLoading } =
    useEditNextCompilationTemplateGroup({ id, onSuccess });
  const [activeView, setActiveView] = useState(
    CompilationTemplateEditorView.Configuration,
  );

  const selectedKind = useWatch({
    control: form.control,
    name: `templates.${SelectedTemplateIndex}.kind`,
  });
  const templateName = useWatch({
    control: form.control,
    name: `templates.${SelectedTemplateIndex}.name`,
  });
  const allFormValues = useWatch({ control: form.control });

  const isArtifacts = selectedKind === CompilationTemplateKind.Artifacts;
  const isOntology = selectedKind === CompilationTemplateKind.Ontology;

  // A model that was never saved has produced nothing, so the compiled-output
  // panel stays disabled until the form carries an id.
  const templateId = useWatch({
    control: form.control,
    name: `templates.${SelectedTemplateIndex}.id`,
  });

  // The service rejects an ontology template whose cross-references do not
  // resolve, and that rejection arrives with nothing on screen (see the schema
  // refinement). Computing the same findings here is what lets the footer say
  // WHY the save is blocked instead of leaving a dead button.
  const entitySection = useWatch({
    control: form.control,
    name: `templates.${SelectedTemplateIndex}.config.entity`,
  }) as OntologySection | undefined;
  const relationSection = useWatch({
    control: form.control,
    name: `templates.${SelectedTemplateIndex}.config.relation`,
  }) as OntologySection | undefined;
  const ontologyIssues = useMemo(
    () =>
      isOntology
        ? validateDraft(draftFromSections(entitySection, relationSection))
        : [],
    [entitySection, relationSection, isOntology],
  );

  /**
   * Undo/redo over the two ontology sections.
   *
   * A snapshot stack keyed on the section value is enough because the canvas has
   * exactly two fields: a per-field history would have to know each field's merge
   * semantics, which promises more than "undo the last canvas edit". Artifacts
   * have their own editors and no history here, so the stack stays empty for them.
   */
  const historyRef = useRef<string[]>([]);
  const [historyCursor, setHistoryCursor] = useState(-1);
  const applyingHistory = useRef(false);

  const sectionsKey = useMemo(
    () => JSON.stringify({ entity: entitySection, relation: relationSection }),
    [entitySection, relationSection],
  );

  useEffect(() => {
    if (!isOntology) {
      historyRef.current = [];
      setHistoryCursor(-1);
      return;
    }
    if (applyingHistory.current) {
      // The change came from undo/redo itself; the stack already points at it.
      applyingHistory.current = false;
      return;
    }
    const stack = historyRef.current;
    if (stack.length === 0) {
      stack.push(sectionsKey);
      setHistoryCursor(0);
      return;
    }
    if (stack[stack.length - 1] === sectionsKey) return;
    // A new edit after an undo drops the branch that is no longer reachable.
    if (historyCursor < stack.length - 1) {
      stack.length = historyCursor + 1;
    }
    stack.push(sectionsKey);
    setHistoryCursor(stack.length - 1);
  }, [sectionsKey, isOntology, historyCursor]);

  const applySnapshot = useCallback(
    (index: number) => {
      const raw = historyRef.current[index];
      if (!raw) return;
      const snapshot = JSON.parse(raw) as {
        entity?: OntologySection;
        relation?: OntologySection;
      };
      applyingHistory.current = true;
      // The form's config is a discriminated union of every template kind, so a
      // concrete section is narrower than the field's type; the watched sections
      // are already asserted to OntologySection for the same reason.
      form.setValue(
        `templates.${SelectedTemplateIndex}.config.entity`,
        snapshot.entity as never,
        { shouldDirty: true },
      );
      form.setValue(
        `templates.${SelectedTemplateIndex}.config.relation`,
        snapshot.relation as never,
        { shouldDirty: true },
      );
      setHistoryCursor(index);
    },
    [form],
  );

  const handleUndo = useCallback(
    () => applySnapshot(historyCursor - 1),
    [applySnapshot, historyCursor],
  );
  const handleRedo = useCallback(
    () => applySnapshot(historyCursor + 1),
    [applySnapshot, historyCursor],
  );

  // Reading `isDirty` subscribes the editor to the form's dirty state; the badge
  // has to appear the moment a field changes, not on the next unrelated render.
  const isDirty = form.formState.isDirty;

  const handleSave = useMemo(
    () => form.handleSubmit(onSubmit),
    [form, onSubmit],
  );

  const handleSubmit = useCallback(
    (event: FormEvent<HTMLFormElement>) => {
      event.stopPropagation();
      handleSave(event);
    },
    [handleSave],
  );

  return (
    <>
      {renderHeader?.({
        isOntology,
        templateId,
        canUndo: isOntology && historyCursor > 0,
        canRedo:
          isOntology &&
          historyCursor >= 0 &&
          historyCursor < historyRef.current.length - 1,
        onUndo: handleUndo,
        onRedo: handleRedo,
        isDirty,
      })}
      <Form {...form}>
        <form className="flex-1 min-h-0 flex flex-col" onSubmit={handleSubmit}>
          <div className="flex min-h-0 flex-1">
            {!sidebarCollapsed && (
              <CompilationTemplateEditorSidebar
                activeView={activeView}
                templateName={templateName}
                onViewChange={setActiveView}
                onCollapse={onSidebarCollapse}
              />
            )}

            <div className="flex min-w-0 flex-1 flex-col">
              {activeView === CompilationTemplateEditorView.Json ? (
                <JsonPreview value={allFormValues} />
              ) : (
                <TemplateConfiguration
                  form={form}
                  builtins={builtins}
                  kindOptions={kindOptions}
                  selectedTemplateIndex={SelectedTemplateIndex}
                >
                  {isArtifacts && (
                    <BlueprintSection
                      form={form}
                      selectedTemplateIndex={SelectedTemplateIndex}
                      builtins={builtins}
                    />
                  )}
                </TemplateConfiguration>
              )}
            </div>
          </div>

          <footer className="shrink-0 px-5 py-4 border-t border-border-button flex items-center justify-end gap-5">
            {ontologyIssues.length > 0 && (
              <span className="mr-auto text-xs text-[#F0A020]">
                {t('knowledgeCompilation.ontologySaveBlocked', {
                  count: ontologyIssues.length,
                  defaultValue:
                    '{{count}} ontology problem(s) must be fixed first — the canvas lists them.',
                })}
              </span>
            )}
            <Button type="button" variant="outline" onClick={onCancel}>
              {cancelText}
            </Button>
            <Button
              type="submit"
              loading={isLoading}
              disabled={ontologyIssues.length > 0}
              className="relative"
            >
              {t('common.save')}
              {/* A save button with no state is indistinguishable from one that is
                  already done — the badge is the difference. */}
              {isDirty && (
                <span
                  className="absolute -top-1 -right-1 size-2 rounded-full bg-state-warning"
                  title={t('knowledgeCompilation.unsavedChanges', {
                    defaultValue: 'Unsaved changes',
                  })}
                />
              )}
            </Button>
          </footer>
        </form>
      </Form>
    </>
  );
}
